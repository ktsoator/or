package transcript

func sequencedForTest(entries ...Entry) []Entry {
	sequenced, err := SequenceEntries(entries, 0)
	if err != nil {
		panic(err)
	}
	return sequenced
}
