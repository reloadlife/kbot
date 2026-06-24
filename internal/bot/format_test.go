package bot

import "testing"

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"frontend-abc-nginx.log": "frontend-abc-nginx.log",
		"pod/../etc.log":         "pod_.._etc.log",
		"":                       "pod.log",
		"a b c.log":              "a_b_c.log",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChunkByLinesUnderLimit(t *testing.T) {
	if got := chunkByLines("hello"); len(got) != 1 || got[0] != "hello" {
		t.Fatalf("expected single chunk, got %v", got)
	}
}

func TestChunkByLinesSplits(t *testing.T) {
	line := ""
	for i := 0; i < 200; i++ {
		line += "0123456789012345678901234\n" // 26 bytes * 200 ≈ 5200 > 4096
	}
	chunks := chunkByLines(line)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > tgMaxLen {
			t.Fatalf("chunk %d exceeds limit: %d", i, len(c))
		}
	}
}
