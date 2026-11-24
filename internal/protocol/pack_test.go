package protocol

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPackFieldsBodyUsesAlbumFields(t *testing.T) {
	body, err := PackFieldsBody([]string{"7628202881140657177"})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Channel string            `json:"book_pack_channel"`
		Fields  []string          `json:"req_fields"`
		IDs     []json.RawMessage `json:"req_ids"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Channel != "inner_feed_detail" {
		t.Fatalf("book_pack_channel = %q", got.Channel)
	}
	// req_ids 必须发成数字：服务端按 int64 解析，字符串会被拒绝。
	if len(got.IDs) != 1 || string(got.IDs[0]) != "7628202881140657177" {
		t.Fatalf("req_ids = %s", string(body))
	}
	for _, want := range []string{"album_id", "album_serial_count", "series_in_album_order", "real_album_serial_count"} {
		found := false
		for _, f := range got.Fields {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("req_fields missing %q: %v", want, got.Fields)
		}
	}
}

func TestParsePackFields(t *testing.T) {
	raw, err := os.ReadFile("testdata/pack-fields.json")
	if err != nil {
		t.Fatal(err)
	}
	infos, err := ParsePackFields(raw)
	if err != nil {
		t.Fatal(err)
	}
	// 第三个 id 没有 album_info，必须被跳过而不是产生零值条目。
	if len(infos) != 2 {
		t.Fatalf("got %d entries, want 2: %v", len(infos), infos)
	}
	second, ok := infos["7628202881140657177"]
	if !ok {
		t.Fatal("missing 7628202881140657177")
	}
	if second.AlbumID != "7624475539960499262" {
		t.Fatalf("album id = %q", second.AlbumID)
	}
	if second.AlbumSerialCount != 16 || second.RealAlbumSerialCount != 14 {
		t.Fatalf("counts = %d/%d", second.AlbumSerialCount, second.RealAlbumSerialCount)
	}
	if second.SeriesInAlbumOrder != 2 {
		t.Fatalf("order = %d", second.SeriesInAlbumOrder)
	}
	if first := infos["7621200213801176126"]; first.SeriesInAlbumOrder != 1 {
		t.Fatalf("first season order = %d", first.SeriesInAlbumOrder)
	}
}

func TestParsePackFieldsRejectsErrorEnvelope(t *testing.T) {
	_, err := ParsePackFields([]byte(`{"code":101001,"message":"content_gone","data":{}}`))
	if err == nil || !strings.Contains(err.Error(), "content_gone") {
		t.Fatalf("err = %v", err)
	}
}
