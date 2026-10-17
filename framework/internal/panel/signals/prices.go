package signals

// The price table the transcript reader prices calls with (PANEL-49 design D5).
// D5 has it copied from the PANEL-48 evidence tool's PRICES and FAST_MULTIPLIER,
// but the tool was not yet committed when this was built: the rows are the pricing
// page's, and task 1.3 compares them with the tool's (tasks.md, Decision log). A model the
// table does not know is UNPRICED: its calls are left out of a cost and counted by
// model, never priced at $0. The table changes only when a model ships, so it is
// code, not config.

import (
	"regexp"
	"strings"
)

const pricingPage = "https://platform.claude.com/docs/en/about-claude/pricing"

// modelPrice is one model's list price, in $ per million tokens. A cache write
// costs 1.25x Input for 5 minutes and 2x for an hour, as the pricing page states for
// every model listed here.
type modelPrice struct {
	Input, Output, CacheRead float64
	// LongOver, where set, is the prompt length (input, cache writes and cache reads)
	// over which the Long rates apply to the whole call, as Haiku 5.5 is priced.
	LongOver                             int64
	LongInput, LongOutput, LongCacheRead float64
	Source                               string
}

// modelPrices is keyed by a model's exact version: claude-opus-5-6 is not Opus 5.5.
var modelPrices = map[string]modelPrice{
	"claude-opus-5-5":   {Input: 4, Output: 20, CacheRead: 0.20, Source: pricingPage + " (Model pricing), read 2026-10-17"},
	"claude-sonnet-5-5": {Input: 2, Output: 10, CacheRead: 0.10, Source: pricingPage + " (Model pricing), read 2026-10-17"},
	"claude-haiku-5-5": {Input: 0.10, Output: 0.50, CacheRead: 0.01,
		LongOver: 100000, LongInput: 0.50, LongOutput: 2.50, LongCacheRead: 0.05,
		Source: pricingPage + " (Model pricing, for prompts up to and over 100,000 tokens), read 2026-10-17"},
}

// fastMultiplier prices fast mode, on every part of a call (caching multipliers
// apply on top of it). A model with no entry has no fast rate, and its fast calls are
// unpriced.
var fastMultiplier = map[string]float64{
	"claude-opus-5-5": 2, // $8 / $40 against $4 / $20: pricingPage (Fast mode pricing), read 2026-10-17
}

var modelDateSuffix = regexp.MustCompile(`-[0-9]{8}$`)

// modelVersion strips a context tag ([1m]) and a date suffix (-20261001) from a
// model id, leaving the version the table is keyed by.
func modelVersion(model string) string {
	return modelDateSuffix.ReplaceAllString(strings.TrimSuffix(model, "[1m]"), "")
}

// callUsage is one call's counts, as the transcript gives them.
type callUsage struct {
	Input, Output int64
	CacheWrite    int64 // every cache write, both durations
	CacheWrite1h  int64 // the part of CacheWrite for an hour
	CacheRead     int64
	Speed         string // "" or "standard", or "fast"
}

// priceCall returns a call's cost in $, and false when the table does not price its
// model or its speed.
func priceCall(model string, u callUsage) (float64, bool) {
	version := modelVersion(model)
	p, ok := modelPrices[version]
	if !ok {
		return 0, false
	}
	mult := 1.0
	switch u.Speed {
	case "", "standard":
	case "fast":
		if mult, ok = fastMultiplier[version]; !ok {
			return 0, false
		}
	default:
		// A speed this table has never seen may carry a premium: it is unpriced
		// rather than priced at the standard rate.
		return 0, false
	}
	in, out, read := p.Input, p.Output, p.CacheRead
	if p.LongOver > 0 && u.Input+u.CacheWrite+u.CacheRead > p.LongOver {
		in, out, read = p.LongInput, p.LongOutput, p.LongCacheRead
	}
	// cache_creation_input_tokens is what is billed; the 1-hour part cannot exceed it.
	hour := min(u.CacheWrite1h, u.CacheWrite)
	cost := float64(u.Input)*in +
		float64(u.CacheWrite-hour)*in*1.25 +
		float64(hour)*in*2 +
		float64(u.CacheRead)*read +
		float64(u.Output)*out
	return cost * mult / 1e6, true
}

// unpricedKey is the name an unpriced call is counted under: the model as written,
// with its speed where the model itself is priced but not at that speed.
func unpricedKey(model, speed string) string {
	if _, ok := modelPrices[modelVersion(model)]; ok && speed != "" && speed != "standard" {
		return model + " (" + speed + ")"
	}
	return model
}
