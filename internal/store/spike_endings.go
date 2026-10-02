package store

// SpikeEnding is the words for one way a spike's run can end. They are kept
// together so a new way is added in one place, and the page, the timeline,
// the findings and the commit message all say it alike.
type SpikeEnding struct {
	// Phrase follows "ended: " or "its run has ended: " in a line of the
	// timeline or the page: "it stopped at its budget".
	Phrase string
	// Lead opens the findings' "How this spike ended" sentence.
	Lead string
	// NotAnswered is the findings' Answer when the run saved none.
	NotAnswered string
	// Run is the sentence on the run's own page (FR-8.4).
	Run string
	// Commit follows "findings (" in the findings' commit message.
	Commit string
}

var spikeEndings = map[string]SpikeEnding{
	SpikeConcluded: {
		Phrase:      "it reached a conclusion",
		Lead:        "The agent reached a conclusion",
		NotAnswered: "Not answered: the run ended without saving an answer.",
		Run:         "It concluded.",
		Commit:      "concluded",
	},
	SpikeBudget: {
		Phrase:      "it stopped at its budget",
		Lead:        "The spike stopped at its budget",
		NotAnswered: "Not answered: the spike stopped at its budget before it reached an answer.",
		Run:         "It stopped at its budget.",
		Commit:      "stopped at the budget",
	},
	SpikeTurnLimit: {
		Phrase:      "it stopped at its turn limit",
		Lead:        "The spike stopped at its turn limit",
		NotAnswered: "Not answered: the spike stopped at its turn limit before it reached an answer.",
		Run:         "It stopped at its turn limit.",
		Commit:      "stopped at the turn limit",
	},
	SpikeTimeBox: {
		Phrase:      "it reached its time box",
		Lead:        "The spike reached the end of its time box",
		NotAnswered: "Not answered: the spike reached the end of its time box before it reached an answer.",
		Run:         "The spike reached the end of its time box.",
		Commit:      "time box ended",
	},
	SpikeFailed: {
		Phrase:      "its run failed",
		Lead:        "The spike's run failed",
		NotAnswered: "Not answered: the spike's run failed before it reached an answer.",
		Run:         "Its run failed.",
		Commit:      "the run failed",
	},
}

// SpikeEndingOf returns the words for an ended_how value. A value this
// version doesn't know reads as a run that is over, with no more said.
func SpikeEndingOf(how string) SpikeEnding {
	if e, ok := spikeEndings[how]; ok {
		return e
	}
	return SpikeEnding{
		Phrase:      "its run is over",
		Lead:        "The spike's run ended",
		NotAnswered: "Not answered: the run ended without saving an answer.",
		Run:         "Its run is over.",
		Commit:      "the run ended",
	}
}
