package sign

import "testing"

// 两档必须真的产生不同的 x-medusa / x-helios：常量、message field 9、
// env 版本串、tag 7 的类型和 field 24 的布局都在 profile 里，漏掉任何一项
// 都会得到一个被服务端静默丢弃的混合体。
func TestPlaybackSignerProfileChangesHeaders(t *testing.T) {
	pairs := [][2]string{
		{"device_platform", "android"},
		{"os", "android"},
		{"ssmix", "a"},
		{"_rticket", ""},
		{"cdid", "7100000000000003"},
		{"aid", "8662"},
		{"version_code", "74332"},
		{"version_name", "7.4.3.32"},
		{"device_id", "7100000000000001"},
		{"iid", "7100000000000002"},
	}
	ent := Entropy{
		NowSecs: 1763460000, MessageRandom: 1234567, Random: 7654321,
		PackedRandom: 0x0badc0de, GorgonRandom: 0x1234, HeliosRandom: 0x5678,
	}
	seen := map[string]string{}
	for _, p := range []*Profile{&ProfileV041003, &ProfileV04604} {
		_, headers, err := (&PlaybackV5Signer{Profile: p}).Sign(pairs, 1763460000000, []byte(`{"a":1}`), &ent)
		if err != nil {
			t.Fatalf("%s: %v", p.Name, err)
		}
		for _, h := range []string{"x-medusa", "x-helios"} {
			if headers[h] == "" {
				t.Fatalf("%s: empty %s", p.Name, h)
			}
			if prev, ok := seen[h]; ok && prev == headers[h] {
				t.Fatalf("%s: %s did not change between profiles", p.Name, h)
			}
			seen[h] = headers[h]
		}
	}
}

// 空 Profile 必须回落到默认档，而不是崩。
func TestPlaybackSignerZeroValueProfile(t *testing.T) {
	var s PlaybackV5Signer
	if got := lookupProfile(s.Profile); got != &DefaultProfile {
		t.Fatalf("zero value resolved to %q", got.Name)
	}
}
