package protocol

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strconv"

	"github.com/beilunyang/openhongguo/sdk/model"
)

const (
	// PathPlaybackV5 是默认档（v04.10.03）的播放模型路径。
	PathPlaybackV5 = "/novel/player/multi_video_model/player/v1"
	// PathPlaybackPreload 是播放前的预加载端点（dr_scene=preload）。
	PathPlaybackPreload = "/novel/player/multi_video_model/preload/v1"
	// PathVideoTab 是播放页的节点列表端点，无请求体；它直接返回
	// data.video_model_data，是当前唯一稳定可用的取源入口。
	PathVideoTab = "/reading/bookapi/video_tab/multi_video_model/v1/"
	// PlaybackVersionCode 与 PlaybackVersionName 是播放链路出示的 App 版本，
	// 统一为 7.4.5.32 / 74532（旧版 7.4.3.32 / 74332）。
	PlaybackVersionCode = 74532
	PlaybackVersionName = "7.4.5.32"
	// VideoTabUserAgentTemplate 是 video_tab 这条 GET 链路用的整机 UA 模板。
	VideoTabUserAgentTemplate = "com.phoenix.read/74532 (Linux; U; Android %s; zh_CN; %s; Build/%s; Cronet/TTNetVersion:8d40f833 2026-03-03 QuicVersion:21ac1950 2025-11-18)"
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
	// 路径带 player 段。dr_scene 已经下放到 preload 预处理，
	// 正式取源不再带（实测带上会让服务端在取源阶段直接返回空体）。
	PlaybackProfileV041003 = PlaybackProfile{
		Name:        "v04.10.03",
		Path:        PathPlaybackV5,
		GzipBody:    true,
		DeviceLevel: 1,
	}

	// PlaybackProfilePreload 是取源前的预加载档：device_level 2 +
	// dr_scene=preload，单独走 preload 路径。
	PlaybackProfilePreload = PlaybackProfile{
		Name:        "preload",
		Path:        PathPlaybackPreload,
		GzipBody:    true,
		DeviceLevel: 2,
		DrScene:     "preload",
	}

	// PlaybackProfileV04604 是 PR #23 报回的那档：明文 body、device_level 4、
	// 路径带尾斜杠、query 带 ts。2026 年实测这档也通（code=0、5 档清晰度），
	// 所以服务端并不像 #23 说的“只接受旧档”；它保留作备选档。
	PlaybackProfileV04604 = PlaybackProfile{
		Name:        "v04.06.04",
		Path:        "/novel/player/multi_video_model/v1/",
		GzipBody:    false,
		DeviceLevel: 4,
		QueryTS:     true,
	}

	// DefaultPlaybackProfile 是运行时的默认档：跟 App 7.4.5.32 自带的
	// 签名库一致，golden vector 可以兑，device_level 1 避开 bytevc2。
	DefaultPlaybackProfile = PlaybackProfileV041003
)

// PlaybackProfileByName 空名返回默认档，未知名字也回退到默认档。
// "preload" 返回预加载档。
func PlaybackProfileByName(name string) PlaybackProfile {
	switch name {
	case PlaybackProfileV04604.Name:
		return PlaybackProfileV04604
	case PlaybackProfilePreload.Name:
		return PlaybackProfilePreload
	}
	return DefaultPlaybackProfile
}

// PlaybackUserAgent is the Cronet User-Agent the 7.4.5.32 client sends with
// the V5 playback request: UserAgent's format with the playback version.
func PlaybackUserAgent(d model.Device) string {
	d = d.OrDefault()
	return fmt.Sprintf("%s/%d (Linux; U; Android %s; zh_CN; %s; Build/%s; Cronet/TTNetVersion:8d40f833 2026-03-03 QuicVersion:21ac1950 2025-11-18)",
		Package, PlaybackVersionCode, d.OSVersion, d.Model, d.Build)
}

// VideoTabUserAgent is the User-Agent the video_tab GET carries; it is
// PlaybackUserAgent built from the same device.
func VideoTabUserAgent(d model.Device) string {
	d = d.OrDefault()
	return fmt.Sprintf(VideoTabUserAgentTemplate, d.OSVersion, d.Model, d.Build)
}

// PlaybackV5Pairs is the V5 playback query in the order the shipped client
// sends it (captured 2026-11-18, version fields bumped to 7.4.5.32), with the profile's device and anonymous
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

// VideoTabPairs 是 video_tab 这条无请求体 GET 的 query。
// device_level 跟着取源档位走（默认 1），不带 dr_scene。
func VideoTabPairs(episodeID string, id SearchIdentity, deviceLevel int) [][2]string {
	d := id.Device.OrDefault()
	version := strconv.Itoa(PlaybackVersionCode)
	return [][2]string{
		{"aid", AID},
		{"app_name", AppName},
		{"version_code", version},
		{"version_name", PlaybackVersionName},
		{"channel", d.Channel},
		{"device_platform", DevicePlatform},
		{"device_type", d.Model},
		{"device_brand", d.Brand},
		{"resolution", d.Resolution},
		{"dpi", strconv.Itoa(d.DPI)},
		{"os_api", strconv.Itoa(d.OSAPI)},
		{"os_version", d.OSVersion},
		{"update_version_code", version},
		{"iid", id.InstallID},
		{"cdid", id.CDID},
		{"device_id", id.DeviceID},
		{"video_id", episodeID},
		{"video_id_type", "0"},
		{"device_level", strconv.Itoa(deviceLevel)},
		{"need_all_video_definition", "1"},
		{"need_mp4_align", "0"},
		{"use_os_player", "0"},
		{"video_platform", "1024"},
		{"source", "0"},
		{"from_video_id", ""},
	}
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
