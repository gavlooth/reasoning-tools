package main

import "math"

const (
	minTokensCap = 1
	maxTokensCap = 200000
)

func clampTemperature(temp float64) float64 {
	if math.IsNaN(temp) || math.IsInf(temp, 0) {
		return 0
	}
	rounded := math.Round(temp*100) / 100
	if rounded < 0 {
		return 0
	}
	if rounded > 1 {
		return 1
	}
	return rounded
}

func clampMaxTokens(tokens int) int {
	if tokens <= 0 {
		return 0
	}
	cfg := GetConfig()
	cap := cfg.MaxTokensCap
	if cap <= 0 {
		cap = maxTokensCap
	}
	if cap < minTokensCap {
		cap = minTokensCap
	}
	if tokens > cap {
		return cap
	}
	return tokens
}
