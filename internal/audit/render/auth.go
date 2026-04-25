package render

func init() {
	register("auth", "login_succeeded", renderAuthLoginSucceeded)
	register("auth", "login_failed", renderAuthLoginFailed)
	register("auth", "password_changed", renderAuthPasswordChanged)
}

func renderAuthLoginSucceeded(before, after []byte) (string, string) {
	return "Login succeeded", ""
}

func renderAuthLoginFailed(before, after []byte) (string, string) {
	return "Login failed", ""
}

func renderAuthPasswordChanged(before, after []byte) (string, string) {
	return "Password changed", ""
}
