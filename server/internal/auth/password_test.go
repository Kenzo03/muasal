package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	h := HashPassword("correct horse battery staple")
	if !CheckPassword(h, "correct horse battery staple") {
		t.Fatal("the right password was rejected")
	}
	if CheckPassword(h, "Correct horse battery staple") {
		t.Fatal("a wrong password was accepted")
	}
	if HashPassword("same") == HashPassword("same") {
		t.Fatal("hashes must be salted")
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=1,t=1,p=1$$", "$bcrypt$x$y$z$w"} {
		if CheckPassword(bad, "x") {
			t.Fatalf("malformed hash %q accepted", bad)
		}
	}
}
