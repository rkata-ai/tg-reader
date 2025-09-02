package telegram

import (
	"context"
	"errors" // Добавляем стандартный пакет для работы с ошибками
	"fmt"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// TermAuth - это простая реализация, которая запрашивает данные из терминала.
type TermAuth struct{}

func (TermAuth) Phone(_ context.Context) (string, error) {
	fmt.Print("Введите номер телефона (например, +79123456789): ")
	var phone string
	_, err := fmt.Scanln(&phone)
	return phone, err
}

func (TermAuth) Password(_ context.Context) (string, error) {
	fmt.Print("Введите пароль: ")
	var password string
	_, err := fmt.Scanln(&password)
	return password, err
}

func (TermAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("Введите код из Telegram: ")
	var code string
	_, err := fmt.Scanln(&code)
	return code, err
}

// AcceptTermsOfService требуется для реализации интерфейса UserAuthenticator.
// Мы просто возвращаем nil, чтобы продолжить авторизацию.
func (TermAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

// SignUp требуется для реализации интерфейса UserAuthenticator.
// Поскольку мы не собираемся создавать нового пользователя,
// мы просто возвращаем ошибку, которая сигнализирует, что пользователь не найден.
func (TermAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("user not found") // Используем стандартную ошибку.
}
