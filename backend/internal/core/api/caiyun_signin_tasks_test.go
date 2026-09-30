package api

import (
	"bytes"
	"image/jpeg"
	"testing"
)

func TestDecodeTaskListV3(t *testing.T) {
	tasks, err := decodeTaskListV3(`{"code":0,"msg":"success","result":[{"id":614,"name":"上传照片","marketname":"sign_in_3","groupid":"beiyong1","state":"WAIT","currstep":2,"stepTypeSet":["sendcloud","nda","click"],"button":{"app":{"canReceive":0}}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != 614 || tasks[0].GroupID != "beiyong1" || tasks[0].CurrStep != 2 {
		t.Fatalf("V3 task decode = %+v", tasks)
	}
	if tasks[0].Enable != nil {
		t.Fatalf("missing enable must remain absent, got %v", *tasks[0].Enable)
	}
	if len(tasks[0].Button) != 1 {
		t.Fatalf("V3 button state was lost: %+v", tasks[0].Button)
	}
	if _, err := decodeTaskListV3(`{"code":403,"msg":"活动已结束"}`); err == nil {
		t.Fatal("business error must not look like an empty task list")
	}
	if _, err := decodeTaskListV3(`{"code":0,"msg":"success"}`); err == nil {
		t.Fatal("missing result must trigger the legacy fallback")
	}
	if _, err := decodeTaskListV3(`<html>error</html>`); err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}

func TestGenerateUniqueSampleJPEG(t *testing.T) {
	first, err := GenerateUniqueSampleJPEG(20, 30)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateUniqueSampleJPEG(20, 30)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("two uploads must not have the same content hash")
	}
	for _, payload := range [][]byte{first, second} {
		if _, err := jpeg.Decode(bytes.NewReader(payload)); err != nil {
			t.Fatalf("JPEG with uniqueness marker is not decodable: %v", err)
		}
	}
}
