package main

import "testing"

func TestProofreadingAppliesWhenEngineFindsNoMisspellings(t *testing.T) {
	draft := "hello world. how are you doing?"
	got, n, err := finishProofreading(draft, draft, 0, nil)
	if err != nil || n == 0 || got != "Hello world. How are you doing?" {
		t.Fatalf("proofreading returned %q, %d, %v", got, n, err)
	}
	if same, n, err := finishProofreading(got, got, 0, nil); err != nil || n != 0 || same != got {
		t.Fatalf("correct draft returned %q, %d, %v", same, n, err)
	}
}

func TestBasicProofreadingOfAlreadyCorrectSpelling(t *testing.T) {
	for _, tc := range [][2]string{
		{"hello world. how are you doing?", "Hello world. How are you doing?"},
		{"i has a appointment. she have an user account.", "I have an appointment. She has a user account."},
		{"i are ready\r\nwe is going", "I am ready\r\nWe are going"},
		{"if he were here, i'd tell him. i dont know", "If he were here, I'd tell him. I don't know"},
		{"  hello dr. smith. i can go", "  Hello dr. smith. I can go"},
		{"5 mg of medicine", "5 mg of medicine"},
		{"visit https://example.com/i-am or email i@example.com. thanks", "Visit https://example.com/i-am or email i@example.com. Thanks"},
		{"i did not go", "I did not go"},
		{"let it do its job. i saw it do that.", "Let it do its job. I saw it do that."},
		{"Ich wohne im Haus.", "Ich wohne im Haus."},
		{"Le livre dont je parle.", "Le livre dont je parle."},
		{"iPhone is off. no. wait.", "iPhone is off. No. Wait."},
	} {
		if got := basicProofread(tc[0]); got != tc[1] {
			t.Errorf("basicProofread(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}
}

func TestOneTapProofreadingAcceptsSmallGrammarChanges(t *testing.T) {
	for _, tc := range [][2]string{
		{"i are tired", "I am tired."},
		{"she have an appointment", "She has an appointment."},
		{"i need a apple", "I need an apple."},
		{"i went to store", "I went to the store."},
		{"i tired", "I am tired."},
		{"i used the the switch", "I used the switch."},
		{"i has a appointment. she have an user account.", "I have an appointment. She has a user account."},
		{"hello. how are you?", "Hello. How are you?"},
		{typoSample, correctedSample},
	} {
		f := &automaticTestField{text: tc[0]}
		got, changed, err := autoCorrect(f, tc[0], func(string) (string, error) { return tc[1], nil }, func() error { return nil })
		if err != nil || !changed || got != tc[1] || f.text != tc[1] {
			t.Errorf("automatic proofread from %q got %q / %v, %v", tc[0], got, changed, err)
		}
	}
}

func TestOneTapProofreadingStillRejectsMeaningChanges(t *testing.T) {
	for _, tc := range [][2]string{
		{"i can go", "I can't go."},
		{"i can go", "I can not go."},
		{"she likes that", "He likes that."},
		{"i was tired", "I am tired."},
		{"i left", "I was left."},
		{"let it do its job", "Let it does its job."},
		{"Press A to continue.", "Press to continue."},
		{"i need 5 mg", "I need 50 mg."},
		{"i need 5 mg", "I need 5 kg."},
		{"please get my sister", "Please get my brother."},
	} {
		if looksLikeProofreading(tc[0], tc[1]) && sameNumbers(tc[0], tc[1]) {
			t.Errorf("accepted meaning change %q -> %q", tc[0], tc[1])
		}
	}
}
