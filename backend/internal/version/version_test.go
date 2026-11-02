package version

import (
	"strings"
	"testing"
)

func TestExecutionAnnotationIsIdempotent(t *testing.T) {
	result := Annotate("任务未完成")
	if !strings.Contains(result, " | build="+Version) || !strings.Contains(result, "commit="+Commit) || Annotate(result) != result {
		t.Fatal(result)
	}
}
