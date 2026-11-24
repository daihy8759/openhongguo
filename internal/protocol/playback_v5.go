package protocol

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strconv"

	"github.com/beilunyang/openhongguo/sdk/model"
)

const (
	PathPlaybackV5      = "/novel/player/multi_video_model/v1/"
	PlaybackVersionCode = 74332
	PlaybackVersionName = "7.4.3.32"
)

// PlaybackProfile 是一档播放协议的线格式（路径 / body 形态 / 设备档位 /
// 是否在 query 里带 ts）。签名档位（sign.Profile）跟它一块选，
// 两者必须一致。
type PlaybackProfile struct {
	// Name 是档位标识，同时用于从选项里反查。
	Name string
	// Path 是播放模型接口的路径。
	Path string
	// GzipBody 为 true 时 body 用 gzip 压缩，并带上 Content-Encoding。
	GzipBody bool
	// DeviceLevel 是 body 里 biz_param.device_level。
	DeviceLevel int
	// DrScene 非空时写进 body（旧版把 dr_scene 放在路径段）。
	DrScene string
	// QueryTS 为 true 时在 query 里带 ts（秒）。
	QueryTS bool
}

var (
	// PlaybackProfileV041003 是 App 7.4.5.32 的档：gzip body、device_level 1、
	// 路径带 player 段。
	PlaybackProfileV041003 = PlaybackProfile{
		Name:        "v04.10.03",
		Path:        "/novel/player/multi_video_model/player/v1",
		GzipBody:    true,
		DeviceLevel: 1,
		DrScene:     "player",
	}

	// PlaybackProfileV04604 是 PR #23 报回的那档：明文 body、device_level 4、
	// 路径带尾斜杠、query 带 ts。
	PlaybackProfileV04604 = PlaybackProfile{
		Name:        "v04.06.04",
		Path:        PathPlaybackV5,
		GzipBody:    false,
		DeviceLevel: 4,
		QueryTS:     true,
	}

	// DefaultPlaybackProfile 是运行时的默认档。
	DefaultPlaybackProfile = PlaybackProfileV04604
)

// PlaybackProfileByName 空名返回默认档，未知名字也回退到默认档。
func PlaybackProfileByName(name string) PlaybackProfile {
	if name == PlaybackProfileV041003.Name {
		return PlaybackProfileV041003
	}
	return DefaultPlaybackProfile
}

// PlaybackUserAgent is the Cronet User-Agent the 7.4.3.32 client sends with
// the V5 playback request: UserAgent's format with the playback version.
func PlaybackUserAgent(d model.Device) string {
	d = d.OrDefault()
	return fmt.Sprintf("%s/%d (Linux; U; Android %s; zh_CN; %s; Build/%s; Cronet/TTNetVersion:8d40f833 2026-03-03 QuicVersion:21ac1950 2025-11-18)",
		Package, PlaybackVersionCode, d.OSVersion, d.Model, d.Build)
}

// PlaybackV5Pairs is the V5 playback query in the order the 7.4.3.32 client
// sends it (captured 2026-11-18), with the profile's device and anonymous
// install. The empty _rticket marks where the signer puts the ticket; order
// is part of the signature.
func PlaybackV5Pairs(p PlaybackProfile, id SearchIdentity) [][2]string {
	d := id.Device.OrDefault()
	kind, pad := "phone", "0"
	if d.Pad {
		kind, pad = "pad", "1"
	}
	version := strconv.Itoa(PlaybackVersionCode)
	base := [][2]string{
		{"device_platform", DevicePlatform},
		{"os", "android"},
		{"ssmix", "a"},
		{"_rticket", ""},
		{"cdid", id.CDID},
		{"channel", d.Channel},
		{"aid", AID},
		{"app_name", AppName},
		{"version_code", version},
		{"version_name", PlaybackVersionName},
		{"manifest_version_code", version},
		{"update_version_code", version},
		{"resolution", d.Resolution},
		{"dpi", strconv.Itoa(d.DPI)},
		{"device_type", d.Model},
		{"device_brand", d.Brand},
		{"language", "zh"},
		{"os_api", strconv.Itoa(d.OSAPI)},
		{"os_version", d.OSVersion},
		{"ac", "wifi"},
		{"host_abi", "arm64-v8a"},
		{"dragon_device_type", kind},
		{"pv_player", version},
		{"compliance_status", "0"},
		{"need_personal_recommend", "1"},
		{"player_so_load", "1"},
		{"is_android_pad_screen", pad},
		{"iid", id.InstallID},
		{"device_id", id.DeviceID},
	}
	if p.QueryTS {
		// 紧跟 _rticket，签名时就地填值，保持客户端参数顺序。
		pairs := make([][2]string, 0, len(base)+1)
		for _, kv := range base {
			pairs = append(pairs, kv)
			if kv[0] == "_rticket" {
				pairs = append(pairs, [2]string{"ts", ""})
			}
		}
		return pairs
	}
	return base
}

// PlaybackV5Body 返回这一档要发送的 body；签名和发送必须用同一份字节。
//
// v04.06.04 档发明文，device_level 4 下发的全是 bytevc1，ffmpeg/mpv 可解；
// v04.10.03 档发 gzip，device_level 1 同样避开 bytevc2（3 会把 1080p 以下
// 的码流全换成 bytevc2，2026-11-18 实测）。
func PlaybackV5Body(p PlaybackProfile, episodeID string) ([]byte, error) {
	biz := map[string]any{
		"detail_page_version": 0, "device_level": p.DeviceLevel, "disable_digg_stat": false,
		"disable_video_relate_book": false, "need_all_video_definition": true,
		"need_mp4_align": false, "use_os_player": false, "use_server_dns": false, "video_platform": 1024,
	}
	payload := map[string]any{
		"biz_param":          biz,
		"mixed_video_id_map": map[string][]string{"1004": {episodeID}},
	}
	if p.DrScene != "" {
		payload["dr_scene"] = p.DrScene
	}
	body, err := jsonMarshal(payload)
	if err != nil {
		return nil, err
	}
	if !p.GzipBody {
		return body, nil
	}
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err = writer.Write(body); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
