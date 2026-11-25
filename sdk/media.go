package sdk

import (
	"context"
	"errors"
	"fmt"

	"github.com/beilunyang/openhongguo/internal/drm"
	"github.com/beilunyang/openhongguo/internal/protocol"
	"github.com/beilunyang/openhongguo/internal/protocol/sign"
	"github.com/beilunyang/openhongguo/sdk/model"
)

// PlaybackOptions 选播放协议与签名的档位。
//
// Profile 留空走默认档（v04.10.03，跟 App 7.4.5.32 的签名库一致）；
// 填 "v04.06.04" 走 PR #23 那档（明文 body、device_level 4、/v1/ 路径）。
type PlaybackOptions struct {
	Profile string
}

// signProfileForPlayback 把播放档位映到签名档位，两档必须一致。
func signProfileForPlayback(p protocol.PlaybackProfile) *sign.Profile {
	if p.Name == protocol.PlaybackProfileV04604.Name {
		return &sign.ProfileV04604
	}
	return &sign.ProfileV041003
}

// ResolvePlayback 加载一集的各清晰度地址、编码和码率。
//
// 取源链路按 App 的顺序走：
//
//	⓪ preload/v1（device_level 2 + dr_scene=preload，只当预热，模型不采用：
//	   该档会把 1080p 以下下发成 bytevc2，ffmpeg/mpv 解不了）
//	① multi_video_model/player/v1（正式取源，device_level 1，不带 dr_scene）
//	② video_model/v1/（旧接口；带 x-argus/x-ladon **和** x-medusa，见 PR #25）
//	③ video_tab（无请求体 GET，最后兜底）
//
// 每一步只要拿到可解析的模型就返回；空体、404/405 继续下一步，
// 其他错误（限流、需要登录等）直接上抛。全部步骤都为空时返回
// ErrAnonymousLimited，让调用方按 Options.DeviceResets 换设备重试。
// 受保护的模型与 ErrDRMProtected 一起返回，可继续交给 ContentKey。
func (c *Client) ResolvePlayback(ctx context.Context, episodeID string, opts PlaybackOptions) (*model.Playback, error) {
	if !validID(episodeID) {
		return nil, fmt.Errorf("invalid episode ID")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	profile := protocol.PlaybackProfileByName(opts.Profile)
	signer := &sign.PlaybackV5Signer{Profile: signProfileForPlayback(profile)}
	return withFreshDevice(ctx, c, true, func(ctx context.Context) (*model.Playback, error) {
		p, err := c.ensureSearchGuest(ctx)
		if err != nil {
			return nil, err
		}
		// emptyErr 是"取源口返回空结果"，routeErr 是"该路径不可用"。
		// 全部步骤都失败时优先上抛 emptyErr：只有它值得换设备重试。
		var emptyErr, routeErr error
		// preload 只当预热：它的 device_level=2 会让服务端把 1080p 以下全换成
		// bytevc2（App 自带解码器能放，ffmpeg/mpv 不能），所以拿到模型也不用。
		_, _ = c.playbackModelRequest(ctx, p, signer, protocol.PlaybackProfilePreload, episodeID)
		// 每一步都返回原始响应；解析失败或缺模型就交给下一步。
		steps := [3]string{"multi_video_model/v1", "video_model/v1", "video_tab"}
		for _, step := range []struct {
			name  string
			fetch func(context.Context, *DeviceProfile) ([]byte, error)
		}{
			{"multi_video_model", func(ctx context.Context, p *DeviceProfile) ([]byte, error) {
				return c.playbackModelRequest(ctx, p, signer, profile, episodeID)
			}},
			{"video_model", func(ctx context.Context, p *DeviceProfile) ([]byte, error) {
				return c.playbackLegacyRequest(ctx, p, episodeID)
			}},
			{"video_tab", func(ctx context.Context, p *DeviceProfile) ([]byte, error) {
				return c.playbackVideoTab(ctx, p, signer, profile.DeviceLevel, episodeID)
			}},
		} {
			raw, err := step.fetch(ctx, p)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if err != nil {
				switch {
				case errors.Is(err, ErrAnonymousLimited):
					if emptyErr == nil {
						emptyErr = err
					}
				case playbackRouteUnavailable(err):
					if routeErr == nil {
						routeErr = err
					}
				default:
					return nil, err
				}
				continue
			}
			playback, parseErr := protocol.ParsePlayback(raw, episodeID)
			if parseErr != nil {
				if emptyErr == nil {
					emptyErr = fmt.Errorf("%w: %s returned no model", ErrAnonymousLimited, step.name)
				}
				continue
			}
			if playback.Protected {
				return playback, ErrDRMProtected
			}
			return playback, nil
		}
		switch {
		case emptyErr != nil:
			// 把"每条路都空"讲清楚：服务端封的是这台设备的取源，换设备未必管用。
			return nil, fmt.Errorf("%w: every playback route (%s → %s → %s) returned an empty body",
				ErrAnonymousLimited, steps[0], steps[1], steps[2])
		case routeErr != nil:
			return nil, routeErr
		}
		return nil, ErrAnonymousLimited
	})
}

// playbackModelRequest 发一次带 body 的 V5 取源请求（preload 或正式取源）。
// body 按档位生成，签名、gzip 与 Content-Encoding 必须和实际发送的字节一致。
func (c *Client) playbackModelRequest(ctx context.Context, p *DeviceProfile, signer *sign.PlaybackV5Signer, profile protocol.PlaybackProfile, episodeID string) ([]byte, error) {
	body, err := protocol.PlaybackV5Body(profile, episodeID)
	if err != nil {
		return nil, err
	}
	ticket := c.clock().UnixMilli()
	query, headers, err := signer.Sign(protocol.PlaybackV5Pairs(profile, c.identity(p)), ticket, body, nil)
	if err != nil {
		return nil, err
	}
	headers = anonymousHeaders(c, p, ticket, headers)
	headers["User-Agent"] = protocol.PlaybackUserAgent(c.deviceOf(p))
	headers["Accept"] = "application/json; charset=utf-8,application/x-protobuf"
	headers["x-xs-from-web"] = "0"
	if profile.GzipBody {
		headers["Content-Encoding"] = "gzip"
	}
	raw, _, err := c.doRaw(ctx, "POST", c.apiOrigin+profile.Path+"?"+query, headers, body)
	return raw, err
}

// playbackVideoTab 是无请求体的 video_tab GET。它是当前唯一稳定返回
// video_model_data 的取源口：签名只带 V5 的 medusa / helios / khronos，
// 无 body 时按 SDK 的空体口径（16 字节全 0）参与哈希，不发 X-SS-STUB。
func (c *Client) playbackVideoTab(ctx context.Context, p *DeviceProfile, signer *sign.PlaybackV5Signer, deviceLevel int, episodeID string) ([]byte, error) {
	ticket := c.clock().UnixMilli()
	query, headers, err := signer.Sign(protocol.VideoTabPairs(episodeID, c.identity(p), deviceLevel), ticket, nil, nil)
	if err != nil {
		return nil, err
	}
	headers = anonymousHeaders(c, p, ticket, headers)
	headers["User-Agent"] = protocol.VideoTabUserAgent(c.deviceOf(p))
	headers["Accept"] = "application/json"
	raw, _, err := c.doRaw(ctx, "GET", c.apiOrigin+protocol.PathVideoTab+"?"+query, headers, nil)
	return raw, err
}

// playbackLegacyRequest 是旧播放接口 video_model/v1/，带 x-argus / x-ladon。
func (c *Client) playbackLegacyRequest(ctx context.Context, p *DeviceProfile, episodeID string) ([]byte, error) {
	legacyBody, err := protocol.PlaybackBody(episodeID)
	if err != nil {
		return nil, err
	}
	ticket := c.clock().UnixMilli()
	query, headers := sign.SignPlaybackPOST(protocol.PathPlayback, protocol.DeviceQueryPairs(c.identity(p)), ticket, p.DeviceID, protocol.VersionName, legacyBody, nil)
	raw, _, err := c.doRaw(ctx, "POST", c.apiOrigin+protocol.PathPlayback+"?"+query, anonymousHeaders(c, p, ticket, headers), legacyBody)
	return raw, err
}

func playbackRouteUnavailable(err error) bool {
	var api *APIError
	return errors.As(err, &api) && (api.HTTPStatus == 404 || api.HTTPStatus == 405)
}

// ContentKey 从播放模型算出加密分集的内容密钥。
// 播放模型就是 ResolvePlayback 连同 ErrDRMProtected 一起返回的那个。这是纯本地计算：
// 按官方播放器的方式从模型的 KeyMaterial 解包出密钥，不再发出任何请求。
// 模型的所有流共用这个密钥（同一集的每种清晰度都带有相同的 spade_a 和 kid，
// 2026-10-24 观察）。
//
// 未加密的模型返回 ErrNotProtected；当前版本无法解包的密钥材料返回
// ErrDRMProtected。
func ContentKey(pb *model.Playback) (model.ContentKey, error) {
	if pb == nil {
		return model.ContentKey{}, fmt.Errorf("playback model is missing")
	}
	if !pb.Protected {
		return model.ContentKey{}, ErrNotProtected
	}
	k, err := drm.Derive(pb.KeyMaterial)
	if err != nil {
		return model.ContentKey{}, fmt.Errorf("%w: %v", ErrDRMProtected, err)
	}
	return model.ContentKey{Hex: k.Hex, Version: k.Version}, nil
}
