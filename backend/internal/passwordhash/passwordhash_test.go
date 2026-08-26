package passwordhash

import "testing"

func TestHashAndVerify(t *testing.T) {
	h, err := Hash("CorrectHorseBatteryStaple123!")
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(h, "CorrectHorseBatteryStaple123!") {
		t.Fatal("correct password did not verify")
	}
	if Verify(h, "WrongPassword123!") {
		t.Fatal("wrong password verified")
	}
}

func TestPasswordLength(t *testing.T) {
	if _, err := Hash("short"); err == nil {
		t.Fatal("short password should fail")
	}
}
