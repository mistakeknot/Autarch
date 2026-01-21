package agent

func advanceOffset(offset int64, lines []string) int64 {
	var n int64
	for _, l := range lines {
		n += int64(len(l)) + 1
	}
	return offset + n
}
