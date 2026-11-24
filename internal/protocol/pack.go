package protocol

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// PackFieldsBody is the book_pack_fields request the shipped client sends to
// DistributionHost. req_fields is narrowed to the album linkage the SDK uses.
func PackFieldsBody(seriesIDs []string) ([]byte, error) {
	ids := make([]any, 0, len(seriesIDs))
	for _, id := range seriesIDs {
		if n, err := strconv.ParseInt(id, 10, 64); err == nil {
			ids = append(ids, n)
			continue
		}
		// 非数字 id 原样按字符串发，服务端会自己拒绝。
		ids = append(ids, id)
	}
	return jsonMarshal(map[string]any{
		"book_pack_channel": "inner_feed_detail",
		"req_fields": []string{
			"album_id", "album_serial_count", "series_in_album_order", "real_album_serial_count",
		},
		"req_ids": ids,
	})
}

// PackAlbumInfo is one series' album linkage as the App reads it.
//
// AlbumSerialCount counts every season slot the album has; RealAlbumSerialCount
// counts the ones that are live — that is the "已上线 N 季" badge. SeriesInAlbumOrder
// is this series' 1-based position in the album.
type PackAlbumInfo struct {
	AlbumID              string
	AlbumSerialCount     int
	RealAlbumSerialCount int
	SeriesInAlbumOrder   int
}

// ParsePackFields reads book_pack_fields and returns the album linkage keyed by
// series ID. Series without an album_info block are left out.
func ParsePackFields(raw []byte) (map[string]PackAlbumInfo, error) {
	var env struct {
		Code    flexString                 `json:"code"`
		Message string                     `json:"message"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if code := env.Code.String(); code != "" && code != "0" {
		msg := env.Message
		if msg == "" {
			msg = "pack fields unavailable"
		}
		return nil, fmt.Errorf("pack fields unavailable: %s (%s)", msg, code)
	}
	out := make(map[string]PackAlbumInfo, len(env.Data))
	for seriesID, payload := range env.Data {
		var one struct {
			AlbumInfo *struct {
				AlbumID     flexString `json:"album_id"`
				SerialCount flexInt    `json:"album_serial_count"`
				RealCount   flexInt    `json:"real_album_serial_count"`
				SeriesOrder flexInt    `json:"series_in_album_order"`
			} `json:"album_info"`
		}
		if err := json.Unmarshal(payload, &one); err != nil {
			continue
		}
		if one.AlbumInfo == nil {
			continue
		}
		info := PackAlbumInfo{
			AlbumID:              one.AlbumInfo.AlbumID.String(),
			AlbumSerialCount:     int(one.AlbumInfo.SerialCount),
			RealAlbumSerialCount: int(one.AlbumInfo.RealCount),
			SeriesInAlbumOrder:   int(one.AlbumInfo.SeriesOrder),
		}
		if info.AlbumID == "" || info.AlbumID == "0" {
			continue
		}
		out[seriesID] = info
	}
	return out, nil
}

// PathSeriesPreload is the series-detail preload the shipped client fires ahead
// of a season switch; it warms the same detail the page then reads.
const PathSeriesPreload = "/novel/player/multi_video_detail/preload/v1"
