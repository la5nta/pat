package fbb

import "testing"

func TestSubjectDecode(t *testing.T) {
	raw := []byte{0x74, 0x65, 0x73, 0x74, 0x20, 0xe6, 0xf8, 0xe5} // test æøå

	// RMS Express compatibility test (encodes subject as ISO-8859-1)
	msg := &Message{Header: Header{"Subject": []string{string(raw)}}}
	if decoded := msg.Subject(); decoded != "test æøå" {
		t.Errorf("Subject with no word-encoding not decoded as ISO-8859-1.")
	}

	msg.Header["Subject"] = []string{"=?utf-8?q?=C2=A1Hola,_se=C3=B1or!?="}
	if decoded := msg.Subject(); decoded != "¡Hola, señor!" {
		t.Errorf("Subject with Q-encoded utf-8 not decoded correctly.")
	}

	msg.Header["Subject"] = []string{"=?ISO-8859-1?q?Test_=E6=F8=E5_abc?="}
	if decoded := msg.Subject(); decoded != "Test æøå abc" {
		t.Errorf("Subject with Q-encoded ISO-8859-1 not decoded correctly.")
	}
}

func TestSubjectEncode(t *testing.T) {
	msg := &Message{Header: make(Header, 1)}

	msg.SetSubject("Test æøå abc")
	if msg.Header["Subject"][0] != "=?ISO-8859-1?q?Test_=E6=F8=E5_abc?=" {
		t.Errorf("Subject not Q-encoded using ISO-8859-1.")
	}

	msg.SetSubject("Test 123 foo bar")
	if msg.Header["Subject"][0] != "Test 123 foo bar" {
		t.Errorf("ASCII-only subject modified on encode.")
	}
}

func TestSubjectRoundtrip(t *testing.T) {
	msg := &Message{Header: make(Header, 1)}

	str := "Hello, 世界"
	msg.SetSubject(str)

	if msg.Subject() != str {
		t.Errorf("Subject encode/decode roundtrip failed.")
	}
}
