package sign

// Profile is one档 of the signing library's wire format. The fields must switch
// as a group: playbackV5License, the magic and the key feed the same sealed
// message, and mixing two档 produces a header the service silently drops
// (HTTP 200 with an empty body).
//
// Two档 are known:
//
//	ProfileV04604  — the older档 the service currently accepts for playback
//	                 (proposed in beilunyang/openhongguo#23)
//	ProfileV041003 — the档 the bundled libmetasec_ml.so in App 7.4.5.32 uses,
//	                 and the one every golden vector in testdata/ was captured
//	                 from
type Profile struct {
	// Name identifies the档 in logs and tests.
	Name string
	// License is x-medusa's message field 6 and an input of x-helios.
	License string
	// Magic is x-medusa's message field 1 and part of the ciphertext prefix.
	Magic []byte
	// SignKey seeds keyHash and derives the AES key and IV.
	SignKey []byte
	// SDKVersion, EnvVersion and SDKCode are message fields 8, env field 6 and
	// message field 9. SDKCode also goes into x-gorgon's RC4 input.
	SDKVersion string
	EnvVersion string
	SDKCode    uint32
	// EnvTag7String makes env field 12's tag 7 a string placeholder.
	// v04.06.04 sends "!notset!"; v04.10.03 sends the integer -999999.
	EnvTag7String bool
	// EnvJSONLegacy drops sts, lp and fl from message field 24.
	EnvJSONLegacy bool
}

var (
	// ProfileV041003 matches libmetasec_ml.so as bundled in App 7.4.5.32.
	ProfileV041003 = Profile{
		Name:       "v04.10.03",
		License:    "646637794",
		Magic:      mustHex("2d4b4fca49750d433fb5ae2c226dcc56"),
		SignKey:    mustHex("b23c2a470a20d105dc3f8c4438f8272ed46d6c89ffdcdedd5108698d26e9489d"),
		SDKVersion: "v04.10.03-ml-android",
		EnvVersion: "v04.10.03.04-bugfix",
		SDKCode:    0x040a0300,
	}

	// ProfileV04604 is the档 beilunyang/openhongguo#23 reports the service
	// still accepts for playback.
	ProfileV04604 = Profile{
		Name:          "v04.06.04",
		License:       "AXYQOS6n2m60x1fVZHIrH3iol",
		Magic:         mustHex("f7e85ffad7d7dc3bd62ac87057cf6118"),
		SignKey:       mustHex("8ebdfa3806ecc5cee79423e6029ed82540bc2218bb7eaef71cb691f7aa8aa2f5"),
		SDKVersion:    "v04.06.04-ml-android",
		EnvVersion:    "v04.06.04.03-bugfix",
		SDKCode:       0x04060400,
		EnvTag7String: true,
		EnvJSONLegacy: true,
	}

	// DefaultProfile signs the ordinary API requests (search, detail, danmaku).
	// It stays on v04.10.03: those endpoints are verified against it and the
	// #23 report only covers playback.
	DefaultProfile = ProfileV041003
)

// lookupProfile returns p, falling back to the default when nil.
func lookupProfile(p *Profile) *Profile {
	if p == nil {
		return &DefaultProfile
	}
	return p
}
