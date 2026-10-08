package compaction

// DecideCall turns the model's probabilities into an action.
func DecideCall(c ToolCall, a CallAnswer, opts ResolvedOptions) Decision {
	d := Decision{ID: c.ID, Tool: c.Tool, CallAnswer: a}
	switch {
	case c.Pinned:
		d.Action, d.Reason = ActionKeep, ReasonPinned
	case a.KeepResult >= opts.KeepThreshold:
		d.Action, d.Reason = ActionKeep, ReasonKept
	case a.KeepCall >= opts.KeepThreshold:
		d.Action, d.Reason = ActionDropResult, ReasonResultDropped
	default:
		d.Action, d.Reason = ActionDropCall, ReasonCallDropped
	}
	return d
}

// unfitDecision keeps a call that was never asked about.
func unfitDecision(c ToolCall) Decision {
	return Decision{
		ID: c.ID, Tool: c.Tool,
		CallAnswer: CallAnswer{KeepCall: 1, KeepResult: 1},
		Action:     ActionKeep, Reason: ReasonUnfit,
	}
}
