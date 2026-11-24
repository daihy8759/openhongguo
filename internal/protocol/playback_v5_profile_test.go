package protocol

import (
	"bytes"
	"compress/gzip"
	"io"
	"strconv"
	"strings"
	"testing"
)

// 两档播放协议并存：v04.10.03 是 App 7.4.5.32 的档，v04.06.04 是
// beilunyang/openhongguo#23 报回的那档。这里把两档的线格式差异钉住，
// 免得以后改一边忘了另一边。
func TestPlaybackProfilesStayDistinct(t *testing.T) {
	id := SearchIdentity{DeviceID: "7100000000000001", InstallID: "7100000000000002", CDID: "7100000000000003"}
	for _, p := range []PlaybackProfile{PlaybackProfileV041003, PlaybackProfileV04604} {
		if p.Name == "" || p.Path == "" {
			t.Fatalf("%q: profile without name or path", p.Name)
		}

		body, err := PlaybackV5Body(p, "7100000000000201")
		if err != nil {
			t.Fatalf("%s: %v", p.Name, err)
		}
		plain := body
		if p.GzipBody {
			zr, err := gzip.NewReader(bytes.NewReader(body))
			if err != nil {
				t.Fatalf("%s: body is not gzip: %v", p.Name, err)
			}
			if plain, err = io.ReadAll(zr); err != nil {
				t.Fatalf("%s: %v", p.Name, err)
			}
		} else if _, err := gzip.NewReader(bytes.NewReader(body)); err == nil {
			t.Fatalf("%s: body is gzip but the profile says plain", p.Name)
		}
		wantLevel := `"device_level":` + strconv.Itoa(p.DeviceLevel)
		if !strings.Contains(string(plain), wantLevel) {
			t.Fatalf("%s: body = %s, want %s", p.Name, plain, wantLevel)
		}
		if p.DrScene == "" && strings.Contains(string(plain), `"dr_scene"`) {
			t.Fatalf("%s: unexpected dr_scene in %s", p.Name, plain)
		}
		if p.DrScene != "" && !strings.Contains(string(plain), `"dr_scene":"`+p.DrScene+`"`) {
			t.Fatalf("%s: missing dr_scene in %s", p.Name, plain)
		}

		pairs := PlaybackV5Pairs(p, id)
		hasTS := false
		for _, kv := range pairs {
			if kv[0] == "ts" {
				hasTS = true
			}
		}
		if hasTS != p.QueryTS {
			t.Fatalf("%s: ts in query = %v, want %v", p.Name, hasTS, p.QueryTS)
		}
	}

	// 两档不能退化成同一组常量。
	if PlaybackProfileV041003.Path == PlaybackProfileV04604.Path {
		t.Fatal("profiles share a path")
	}
	if PlaybackProfileV041003.DeviceLevel == PlaybackProfileV04604.DeviceLevel {
		t.Fatal("profiles share a device level")
	}
	if PlaybackProfileV041003.GzipBody == PlaybackProfileV04604.GzipBody {
		t.Fatal("profiles share a body encoding")
	}
}

func TestPlaybackProfileByName(t *testing.T) {
	if got := PlaybackProfileByName(""); got != DefaultPlaybackProfile {
		t.Fatalf("empty name = %q", got.Name)
	}
	if got := PlaybackProfileByName(PlaybackProfileV041003.Name); got != PlaybackProfileV041003 {
		t.Fatalf("v04.10.03 name = %q", got.Name)
	}
	if got := PlaybackProfileByName("nope"); got != DefaultPlaybackProfile {
		t.Fatalf("unknown name = %q", got.Name)
	}
}
