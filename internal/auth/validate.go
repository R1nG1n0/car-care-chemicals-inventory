package auth

import (
	"regexp"
	"unicode/utf8"
)

// Правила проверки ввода из docs/API.md (ТБ-7).

var loginRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

// ValidateLogin: 3–32 символа, латиница, цифры, «_ . -».
func ValidateLogin(login string) error {
	if !loginRe.MatchString(login) {
		return &ValidationError{Field: "login", Message: "Логин — от 3 до 32 символов: латиница, цифры, «_», «.», «-»"}
	}
	return nil
}

// ValidatePassword: 8–128 символов (считаются символы, а не байты).
func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || n < 8 || n > 128 {
		return &ValidationError{Field: "password", Message: "Пароль — от 8 до 128 символов"}
	}
	return nil
}

// ValidateSettings: шаги 1–1000 и 1–5000, малый меньше большого.
func ValidateSettings(st Settings) error {
	if st.StepSmall < 1 || st.StepSmall > 1000 {
		return &ValidationError{Field: "step_small", Message: "Малый шаг — целое число от 1 до 1000"}
	}
	if st.StepLarge < 1 || st.StepLarge > 5000 {
		return &ValidationError{Field: "step_large", Message: "Большой шаг — целое число от 1 до 5000"}
	}
	if st.StepSmall >= st.StepLarge {
		return &ValidationError{Field: "step_large", Message: "Большой шаг должен быть больше малого"}
	}
	return nil
}
