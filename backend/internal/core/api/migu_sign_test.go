package api

import (
	"strings"
	"testing"
)

func TestMiguSignReferenceVectors(t *testing.T) {
	binary := make([]byte, 256)
	for i := range binary {
		binary[i] = byte(i)
	}
	// Expected digests from the supplied Python reference, including exact
	// block boundaries and signed bytes; not Go-generated golden outputs.
	for _, tc := range []struct {
		name, input, key, want string
	}{
		{"hello", "hello", strings.Repeat("BB", 40), "11D73CF9BD4A9C2EFB1D131245D03F24"},
		{"stream", "helloworld", strings.Repeat("CC", 40), "E28D3870A04A7A22B899D91EBD5DE911"},
		{"empty", "", miguCertificatePrefix, "330D403813C5F7041929D523A6CB22D9"},
		{"exact block", strings.Repeat("a", 32), miguCertificatePrefix, "7D4664141EB692A310EA585001FB9843"},
		{"partial block", strings.Repeat("a", 33), miguCertificatePrefix, "DD91866B8E99376BF290115EB8261B87"},
		{"two blocks", strings.Repeat("a", 96), miguCertificatePrefix, "4871C1E51CD4153F9145D09DA3BAFA32"},
		{"binary", string(binary), miguCertificatePrefix, "57FA155E33264B79BEE1FC6B1D9143CF"},
		{"get", "/act-task/query-tasksABC1794758400000", miguCertificatePrefix, "6427F8A3E9A857D4EF7BB468599AB8DF"},
		{"post", `/act-task/task-reportABC1794758400000{"taskId":"qsq"}`, miguCertificatePrefix, "F52195F1398EAFCD9A58652CCD6CB692"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := miguSign([]byte(tc.input), tc.key); got != tc.want {
				t.Fatalf("sign = %s, want %s", got, tc.want)
			}
		})
	}
}
