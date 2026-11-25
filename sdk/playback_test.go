package sdk_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/beilunyang/openhongguo/internal/protocol"
	"github.com/beilunyang/openhongguo/sdk"
)

// 取源链路的顺序与回落：preload → video_tab → multi_video_model → 旧接口。
// 服务端 2026-11 起对匿名设备的 POST 取源只回空体，video_tab 是实际可用的那一步，
// 所以链路必须能一路回落到它，并且只把空体当成“可以换设备重试”的失败。

func playbackChainHandler(t *testing.T, seen *[]captured, models map[string][]byte) http.Handler {
	t.Helper()
	reg := fixture(t, "register-response.json")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, captured{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Header: r.Header.Clone()})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == protocol.PathRegister {
			_, _ = w.Write(reg)
			return
		}
		if body, ok := models[r.URL.Path]; ok {
			_, _ = w.Write(body)
			return
		}
		// 其余取源口：HTTP 200 + 空体，和线上匿名设备的实际响应一致。
		w.WriteHeader(http.StatusOK)
	})
}

func TestPlaybackFallsBackToVideoTab(t *testing.T) {
	var seen []captured
	handler := playbackChainHandler(t, &seen, map[string][]byte{
		protocol.PathVideoTab: fixture(t, "video-tab-playback.json"),
	})
	client, _ := testClient(t, handler)
	playback, err := client.ResolvePlayback(context.Background(), "7100000000000301", sdk.PlaybackOptions{})
	if !errors.Is(err, sdk.ErrDRMProtected) {
		t.Fatalf("err = %v", err)
	}
	if playback == nil || len(playback.Streams) != 1 || !playback.Protected {
		t.Fatalf("playback = %+v", playback)
	}
	if got := playback.Streams[0].Definition; got != "360p" {
		t.Fatalf("definition = %s", got)
	}
	if playback.EpisodeID != "7100000000000301" {
		t.Fatalf("episode = %s", playback.EpisodeID)
	}

	var order []string
	for _, hit := range seen {
		switch hit.Path {
		case protocol.PathRegister:
		case protocol.PathPlaybackPreload, protocol.PathVideoTab, protocol.PathPlaybackV5, protocol.PathPlayback:
			order = append(order, hit.Path)
		default:
			t.Fatalf("unexpected path %s", hit.Path)
		}
	}
	// preload 只当预热（在链路外单独发一次），正式链路是 model → tab。
	want := []string{protocol.PathPlaybackPreload, protocol.PathPlaybackV5, protocol.PathPlayback, protocol.PathVideoTab}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("chain order = %v, want %v", order, want)
	}

	tab := seen[len(seen)-1]
	q, _ := url.ParseQuery(tab.RawQuery)
	if tab.Method != http.MethodGet || q.Get("device_level") != "1" || q.Get("version_code") != "74532" || q.Get("video_id") != "7100000000000301" {
		t.Fatalf("video_tab request = %s %s", tab.Method, tab.RawQuery)
	}
	if q.Get("dr_scene") != "" {
		t.Fatalf("video_tab carries dr_scene: %s", tab.RawQuery)
	}
	// video_tab 只带 V5 的 Medusa / Helios / Khronos，不额外带旧签名头。
	if tab.Header.Get("x-medusa") == "" || tab.Header.Get("x-helios") == "" || tab.Header.Get("x-khronos") == "" {
		t.Fatalf("video_tab signature headers = %v", tab.Header)
	}
	if tab.Header.Get("X-SS-Stub") != "" || tab.Header.Get("Content-Type") != "" {
		t.Fatalf("video_tab carried a body stub: %v", tab.Header)
	}
}

// preload 的模型按 device_level=2 下发（1080p 以下是 bytevc2，ffmpeg 解不了），
// 所以它只当预热：即使答了模型也不能短路正式链路。
func TestPreloadModelIsNotUsedAsSource(t *testing.T) {
	var seen []captured
	handler := playbackChainHandler(t, &seen, map[string][]byte{
		protocol.PathPlaybackPreload: fixture(t, "clear-playback.json"),
		protocol.PathVideoTab:        fixture(t, "video-tab-playback.json"),
	})
	client, _ := testClient(t, handler)
	playback, err := client.ResolvePlayback(context.Background(), "7100000000000301", sdk.PlaybackOptions{})
	if !errors.Is(err, sdk.ErrDRMProtected) {
		t.Fatalf("err = %v", err)
	}
	if playback == nil || len(playback.Streams) != 1 || playback.Streams[0].Definition != "360p" {
		t.Fatalf("playback = %+v", playback)
	}
	var paths []string
	for _, hit := range seen {
		if hit.Path == protocol.PathRegister {
			continue
		}
		paths = append(paths, hit.Path)
	}
	want := []string{protocol.PathPlaybackPreload, protocol.PathPlaybackV5, protocol.PathPlayback, protocol.PathVideoTab}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("chain = %v, want %v", paths, want)
	}
}

func TestPlaybackAllRoutesEmptyIsAnonymousLimited(t *testing.T) {
	var seen []captured
	client, _ := testClient(t, playbackChainHandler(t, &seen, nil))
	_, err := client.ResolvePlayback(context.Background(), "7100000000000301", sdk.PlaybackOptions{})
	if !errors.Is(err, sdk.ErrAnonymousLimited) {
		t.Fatalf("err = %v", err)
	}
}
