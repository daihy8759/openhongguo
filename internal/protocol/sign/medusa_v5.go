package sign

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

// MedusaEnv is the device environment the v04.10.03 SDK reports inside
// x-medusa. One device keeps the same values across requests; PID, PPID and
// LaunchMillis describe the app process, and the clock offsets are added to
// the request timestamp.
//
// DefaultMedusaEnv returns the values the SDK reports under the unidbg
// emulator (openhongguo-signer), except Status, which the service only
// accepts with a real device's value.
type MedusaEnv struct {
	PID, PPID    int64
	LaunchMillis int64
	// Clock18, Clock19 and Clock20 are environment fields 18-20 minus the
	// request timestamp (three clocks the SDK samples).
	Clock18, Clock19, Clock20 int64
	Channel                   string
	// Fields of the JSON blob (message field 24).
	UnH, Fkd, Pd int64
	Sts          int64
	Lp, Fl       string
	// Status is message field 21. The SDK reports 369 on a real device and
	// 377 under the emulator; since 2026-11-23 the service answers playback
	// requests that carry 377 with an empty body.
	Status int64
	// Device switches the message to a real device's layout; nil keeps the
	// emulator's.
	Device *MedusaDevice
}

// Message field 21 as the SDK reports it on a real device (captured from
// the 7.4.5.32 app on 2026-11-23) and under the unidbg emulator.
const (
	medusaDeviceStatus   = 369
	medusaEmulatorStatus = 377
)

// DefaultMedusaEnv returns an environment shaped like the emulator's, with
// the process fields set from the given launch time and the status of a
// real device.
func DefaultMedusaEnv(launchMillis int64) MedusaEnv {
	return MedusaEnv{
		PID: 4321, PPID: 1234,
		LaunchMillis: launchMillis,
		Clock18:      766353820, Clock19: 16, Clock20: -1072050540,
		Channel: "xiaomi_8662_64",
		// field 24 的 un_h / fkd / pd。它们和设备状态绑定、服务端无法校验，
		// 不随签名档位变化；这一组取自 PR #23 的真机抓包。
		UnH: 1879194040, Sts: 27, Fkd: 3672518972, Pd: -1872573247,
		Lp:     "2|520830913092|521433182354|2078052786776|1081701896",
		Fl:     "0|0|2523408538|2586339478|997724116|0|0|0|0|606600138",
		Status: medusaDeviceStatus,
	}
}

// MedusaRandom holds the three rand() values the SDK draws per request.
type MedusaRandom struct {
	Message uint32 // message field 3
	Key     uint32 // key hash seed; its bytes also go in the header and trailer
	Packed  uint32 // the word after the packed header byte
}

const (
	medusaV5Version    = 5
	medusaV5PackedHead = 0xca
)

// medusaV5Input is what a request contributes to x-medusa.
type medusaV5Input struct {
	querySM3 [32]byte
	bodyMD5  [16]byte
	digest   [20]byte // message field 13
	ts       uint32
}

// newMedusaV5Input hashes the request. query is the query string that is
// sent (without '?'); body is nil for a request without one. It returns
// false when the request selects a field-13 variant this package lacks;
// the caller then changes _rticket and tries again.
func newMedusaV5Input(query string, body []byte, ts uint32) (medusaV5Input, bool) {
	in := medusaV5Input{querySM3: sm3Sum([]byte(query)), ts: ts}
	if body != nil {
		in.bodyMD5 = md5Raw(body)
	}
	digest, ok := f13V5(in.querySM3, in.bodyMD5, ts)
	in.digest = digest
	return in, ok
}

// assembleMedusaV5 builds x-medusa with the given profile.
func assembleMedusaV5(p *Profile, in medusaV5Input, deviceID string, env MedusaEnv, rnd MedusaRandom) string {
	p = lookupProfile(p)
	return sealMedusaV5(p, medusaV5Message(p, in, deviceID, env, rnd), in, rnd, env.flags())
}

func medusaV5Message(p *Profile, in medusaV5Input, deviceID string, env MedusaEnv, rnd MedusaRandom) []byte {
	ts := in.ts
	const unset = 694367 // message field 15's counters when the SDK has not set them
	counters := concat(protoSInt(1, unset), protoSInt(2, unset), protoSInt(3, unset), protoSInt(5, unset))
	var appVersion []byte
	if env.Device != nil {
		counters = concat(protoSInt(1, nextMedusaCount()), protoSInt(2, 5), protoSInt(3, unset), protoSInt(5, unset))
		appVersion = protoString(7, env.Device.AppVersion)
	}
	return concat(
		protoBytes(1, p.Magic),
		protoSInt(2, medusaV5Version),
		protoSInt(3, int64(rnd.Message)),
		protoString(4, appID),
		protoString(5, deviceID),
		protoString(6, p.License),
		appVersion,
		protoString(8, p.SDKVersion),
		protoSInt(9, int64(p.SDKCode)),
		protoBytes(10, env.flags()),
		protoSInt(12, int64(ts)),
		protoBytes(13, in.digest[:]),
		protoBytes(14, in.querySM3[:6]),
		protoBytes(15, counters),
		protoSInt(17, int64(ts)),
		protoString(20, "none"),
		protoSInt(21, env.Status),
		protoBytes(23, medusaV5Env(p, ts, deviceID, env, rnd)),
		protoString(24, medusaV5JSON(p, env)),
	)
}

func medusaV5Env(p *Profile, ts uint32, deviceID string, env MedusaEnv, rnd MedusaRandom) []byte {
	if d := env.Device; d != nil {
		uptime := int64(ts) - env.LaunchMillis/1000
		if uptime < 1 {
			uptime = 1
		}
		version := d.AppVersion
		if version == "" {
			version = notSet
		}
		return concat(
			protoSInt(1, uptime),
			protoString(6, p.EnvVersion),
			protoSInt(7, env.PID),
			protoString(8, env.Channel),
			protoBytes(12, medusaV5DeviceBlock(deviceID, env, int64(ts)*1000+int64(rnd.Message%1000))),
			protoEmpty(13),
			protoString(14, version),
			protoSInt(15, env.PPID),
			protoSInt(18, int64(ts)+env.Clock18),
			protoSInt(19, int64(ts)+env.Clock19),
			protoSInt(20, int64(ts)+env.Clock20),
		)
	}
	return concat(
		protoString(6, p.EnvVersion),
		protoSInt(7, env.PID),
		protoString(8, env.Channel),
		protoBytes(12, medusaV5Device(p, deviceID, env)),
		protoEmpty(13),
		protoString(14, notSet),
		protoSInt(15, env.PPID),
		protoSInt(18, int64(ts)+env.Clock18),
		protoSInt(19, int64(ts)+env.Clock19),
		protoSInt(20, int64(ts)+env.Clock20),
	)
}

const (
	notSet   = "!notset!"
	unsetInt = -999999
)

// medusaV5Device is environment field 12. Fields the emulator cannot answer
// carry the SDK's placeholders: "!notset!" and -999999. Which placeholder tag
// 7 takes depends on the profile.
func medusaV5Device(p *Profile, deviceID string, env MedusaEnv) []byte {
	out := concat(protoSInt(1, 1), protoString(3, appID), protoString(4, deviceID))
	for tag := uint32(5); tag <= 40; tag++ {
		switch {
		case tag == 7 && p.EnvTag7String:
			out = append(out, protoString(tag, notSet)...)
		case tag == 5 || tag == 6 || (tag >= 11 && tag <= 13) || tag == 15 || tag == 22 || (tag >= 30 && tag <= 37):
			out = append(out, protoString(tag, notSet)...)
		case (tag >= 7 && tag <= 10) || tag == 14 || (tag >= 23 && tag <= 25) || tag == 27 || tag == 29 || tag == 38:
			out = append(out, protoSInt(tag, unsetInt)...)
		case tag >= 16 && tag <= 21:
			out = append(out, protoFloat(tag, unsetInt)...)
		case tag == 26 || tag == 28 || tag == 40:
			out = append(out, protoSInt(tag, env.LaunchMillis)...)
		}
	}
	return out
}

// medusaV5JSON is message field 24. The v04.06.04 profile sends the shorter
// form: no sts / lp / fl. The numbers come from the device environment.
func medusaV5JSON(p *Profile, env MedusaEnv) string {
	if p.EnvJSONLegacy {
		return fmt.Sprintf(`{"cmr":16777216,"cmr2":16777216,"un_h":%d,"vpn":0,"kd":0,"fkd":%d,"pd":%d,"dyn":"","do":0,"tk":true}`,
			env.UnH, env.Fkd, env.Pd)
	}
	return fmt.Sprintf(`{"cmr":16777216,"cmr2":16777216,"un_h":%d,"vpn":0,"sts":%d,"kd":0,"fkd":%d,"pd":%d,"lp":%s,"fl":%s,"dyn":"","do":0,"tk":true}`,
		env.UnH, env.Sts, env.Fkd, env.Pd, strconv.Quote(env.Lp), strconv.Quote(env.Fl))
}

// sealMedusaV5 encrypts a message into the x-medusa wire format.
func sealMedusaV5(p *Profile, message []byte, in medusaV5Input, rnd MedusaRandom, flags []byte) string {
	ts := in.ts
	hash, seed := keyHash(p.SignKey, rnd.Key)
	transformed := append(append([]byte(nil), flags...), xmxor(message, hash[:])...)
	for i, j := 0, len(transformed)-1; i < j; i, j = i+1, j-1 {
		transformed[i], transformed[j] = transformed[j], transformed[i]
	}
	for i := range transformed {
		transformed[i] ^= seed[(^i)&3]
	}
	check := uint32(0x18000001) | uint32(in.querySM3[0]&63)<<14 | uint32(in.digest[0]&63)<<8
	packed := concat([]byte{medusaV5PackedHead}, le32(rnd.Packed), le32(check), transformed, []byte{byte(rnd.Key >> 16), byte(rnd.Key >> 24)})
	key := md5Raw(p.SignKey[:16])
	iv := md5Raw(p.SignKey[16:])
	encrypted := encryptPlaybackV5(packed, key[:], iv[:], ts)
	prefix := concat(le32(medusaV5Version), p.Magic)
	for i := range prefix {
		prefix[i] ^= byte(ts >> (uint(i&3) * 8))
	}
	return base64.StdEncoding.EncodeToString(concat(prefix, []byte{byte(rnd.Key), byte(rnd.Key >> 8), 0, 1}, encrypted))
}

func protoEmpty(tag uint32) []byte {
	return append(protoKey(tag, 2), 0)
}
