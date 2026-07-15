package ai

// budgetSafety is a token cushion below the model's window for tokenizer
// estimation error and framing overhead.
const budgetSafety = 512

// Budget trims a request to fit a model's context window while protecting the
// cache-friendly prefix. It keeps the (cached) System — truncating only its
// tail (the world bible) if enormous, never the leading instructions — keeps
// the most recent conversation turns, always retains the final message
// (truncating its *front*, i.e. injected context, if it alone is too big so the
// author's question at the end survives), and starts the kept run with a user
// turn (Anthropic requires it).
//
// contextWindow is the model's total window; outputReserve is held back for the
// reply.
func Budget(req Request, contextWindow, outputReserve int) Request {
	avail := contextWindow - outputReserve - budgetSafety
	if avail < 512 {
		avail = 512
	}

	// Cap the system prompt so the world bible can't consume the whole window;
	// truncate its tail (instructions live at the top).
	sysCap := avail * 3 / 4
	if EstimateTokens(req.System) > sysCap {
		req.System = truncateToTokens(req.System, sysCap) +
			"\n…(world context truncated to fit this model's context window)"
	}

	fixed := EstimateTokens(req.System)
	for _, t := range req.Tools {
		fixed += EstimateTokens(t.Name) + EstimateTokens(t.Description) + EstimateTokens(string(t.Schema)) + 8
	}
	budget := avail - fixed
	if budget < 128 {
		budget = 128
	}

	msgs := append([]Message(nil), req.Messages...)
	if len(msgs) == 0 {
		return req
	}

	// The final message is mandatory. If it alone exceeds the budget, truncate
	// its front — the injected context — keeping the author's question at the end.
	fi := len(msgs) - 1
	if EstimateTokens(msgs[fi].Content) > budget {
		msgs[fi].Content = truncateFrontToTokens(msgs[fi].Content, budget)
	}
	used := msgTokens(msgs[fi])
	start := fi
	for i := fi - 1; i >= 0; i-- {
		if used+msgTokens(msgs[i]) > budget {
			break
		}
		used += msgTokens(msgs[i])
		start = i
	}
	kept := msgs[start:]
	for len(kept) > 1 && kept[0].Role != RoleUser {
		kept = kept[1:]
	}
	req.Messages = kept
	return req
}

func msgTokens(m Message) int {
	t := EstimateTokens(m.Content) + 4
	for _, tc := range m.ToolCalls {
		t += EstimateTokens(tc.Name) + EstimateTokens(tc.Arguments) + 8
	}
	return t
}

// truncateToTokens truncates s to about tokens tokens (rune-safe).
func truncateToTokens(s string, tokens int) string {
	maxRunes := tokens * 35 / 10 // ~3.5 chars/token
	r := []rune(s)
	if maxRunes < 0 {
		maxRunes = 0
	}
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

// truncateFrontToTokens keeps about the last tokens tokens of s.
func truncateFrontToTokens(s string, tokens int) string {
	maxRunes := tokens * 35 / 10
	r := []rune(s)
	if maxRunes < 0 {
		maxRunes = 0
	}
	if len(r) <= maxRunes {
		return s
	}
	return "…(earlier context truncated)\n" + string(r[len(r)-maxRunes:])
}
