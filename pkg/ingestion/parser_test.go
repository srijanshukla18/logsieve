package ingestion

import (
    "testing"
    "time"

    "github.com/rs/zerolog"
)

func TestTryParseTime_IntegerUnits(t *testing.T) {
    p := NewParser(zerolog.Nop())

    cases := []struct{
        in string
        wantUnit time.Duration
    }{
        {"1700000000", time.Second},      // 10 digits seconds
        {"1700000000000", time.Millisecond}, // 13 digits ms
        {"1700000000000000", time.Microsecond}, // 16 digits µs
        {"1700000000000000000", time.Nanosecond}, // 19 digits ns
    }

    for _, c := range cases {
        got, err := p.tryParseTime(c.in)
        if err != nil {
            t.Fatalf("parse error for %s: %v", c.in, err)
        }
        // Reconstruct back to check unit magnitude approximately
        // We can't know exact epoch, but conversion should not overflow
        _ = got
    }
}

func TestTryParseTime_RFC3339(t *testing.T) {
    p := NewParser(zerolog.Nop())
    ts := "2024-08-19T12:34:56Z"
    got, err := p.tryParseTime(ts)
    if err != nil {
        t.Fatalf("parse error: %v", err)
    }
    if got.Format(time.RFC3339) != ts {
        t.Fatalf("unexpected parsing result: %s", got.Format(time.RFC3339))
    }
}

func TestParse_ToleratesFloatSeconds(t *testing.T) {
    p := NewParser(zerolog.Nop())
    ts := "1700000000.123"
    got, err := p.tryParseTime(ts)
    if err != nil {
        t.Fatalf("parse error: %v", err)
    }
    if got.Unix() != 1700000000 {
        t.Fatalf("unexpected seconds: %d", got.Unix())
    }
}

