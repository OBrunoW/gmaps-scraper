package hashid

import (
	"testing"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
)

const defaultSalt = "gmaps-scraper-pro-default-salt"

var goldenCases = []struct {
	id     int64
	public string
}{
	{1, "RrBLGvxaVAjoO9wlbpl1Jz3NynWQgek6"},
	{2, "bQknvX7OVolGW40VL2ZEBNDAjRdgaMK3"},
	{42, "NXgRAzrMGm3BaKpagwWkd74jJYynPEvL"},
	{1000, "aogdX9LW4kYZbz0PK1p5yDBE3qQMeVAx"},
	{999999, "dyGgX1OLJNjx72GW1QW2vDeVQnW49zbM"},
	{123456789012, "aYRnJZyqxBgpWZx8vKgd2AQDl7MKk8Le"},
	{9007199254740993, "qNo8mDQMlW2MnP9979ByYbp6vLZGJa4V"},
}

func newDefaultCodec(t *testing.T) *Codec {
	t.Helper()
	codec, err := NewCodec(defaultSalt)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return codec
}

func TestCodecImplementsPort(t *testing.T) {
	var _ domain.JobIDCodec = (*Codec)(nil)
}

func TestCodecEncodeMatchesGolden(t *testing.T) {
	codec := newDefaultCodec(t)

	for _, tt := range goldenCases {
		if got := codec.Encode(tt.id); got != tt.public {
			t.Errorf("Encode(%d) = %q, want %q", tt.id, got, tt.public)
		}
	}
}

func TestCodecDecodeMatchesGolden(t *testing.T) {
	codec := newDefaultCodec(t)

	for _, tt := range goldenCases {
		got, err := codec.Decode(tt.public)
		if err != nil {
			t.Errorf("Decode(%q): %v", tt.public, err)
			continue
		}
		if got != tt.id {
			t.Errorf("Decode(%q) = %d, want %d", tt.public, got, tt.id)
		}
	}
}

func TestCodecDecodeInvalid(t *testing.T) {
	codec := newDefaultCodec(t)

	for _, input := range []string{"", "!!!", "not a valid id"} {
		if _, err := codec.Decode(input); err == nil {
			t.Errorf("Decode(%q) should fail", input)
		}
	}
}

func TestCodecDifferentSaltProducesDifferentID(t *testing.T) {
	other, err := NewCodec("another-salt")
	if err != nil {
		t.Fatal(err)
	}

	if other.Encode(42) == newDefaultCodec(t).Encode(42) {
		t.Error("different salts should not produce the same public id")
	}
}
