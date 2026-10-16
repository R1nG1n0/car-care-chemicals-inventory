package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
)

// runUser — подкоманды user add | passwd | list (ФТ-1.2).
func runUser(ctx context.Context, svc *auth.Service, args []string) error {
	if len(args) == 0 {
		return errors.New("нужна подкоманда: user add <логин> [-owner], user passwd <логин> или user list")
	}
	sub, rest := args[0], args[1:]

	switch sub {
	case "add":
		owner := false
		var logins []string
		for _, a := range rest {
			switch a {
			case "-owner", "--owner":
				owner = true
			default:
				if strings.HasPrefix(a, "-") {
					return fmt.Errorf("user add: неизвестный флаг %s", a)
				}
				logins = append(logins, a)
			}
		}
		if len(logins) != 1 {
			return errors.New("использование: user add <логин> [-owner]")
		}
		if err := auth.ValidateLogin(logins[0]); err != nil {
			return userError(err)
		}
		password, err := readPassword()
		if err != nil {
			return err
		}
		u, err := svc.CreateUser(ctx, logins[0], password, owner)
		if err != nil {
			return userError(err)
		}
		role := "пользователь"
		if u.IsOwner {
			role = "владелец"
		}
		fmt.Printf("Создан пользователь %s (%s)\n", u.Login, role)
		return nil

	case "passwd":
		if len(rest) != 1 {
			return errors.New("использование: user passwd <логин>")
		}
		u, err := svc.UserByLogin(ctx, rest[0])
		if errors.Is(err, auth.ErrNotFound) {
			return fmt.Errorf("пользователь %s не найден", rest[0])
		}
		if err != nil {
			return err
		}
		password, err := readPassword()
		if err != nil {
			return err
		}
		if err := svc.SetPassword(ctx, u.ID, password, ""); err != nil {
			return userError(err)
		}
		fmt.Printf("Пароль пользователя %s изменён, все его сессии сброшены\n", u.Login)
		return nil

	case "list":
		if len(rest) != 0 {
			return errors.New("использование: user list")
		}
		users, err := svc.ListUsers(ctx)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			fmt.Println("Пользователей нет. Создайте владельца: garage user add <логин>")
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ЛОГИН\tВЛАДЕЛЕЦ\tСОЗДАН")
		for _, u := range users {
			owner := "нет"
			if u.IsOwner {
				owner = "да"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", u.Login, owner, u.CreatedAt.Local().Format("2006-01-02 15:04"))
		}
		return tw.Flush()
	}
	return fmt.Errorf("неизвестная подкоманда user %s", sub)
}

// userError показывает ошибку проверки ввода человеческим текстом.
func userError(err error) error {
	var ve *auth.ValidationError
	switch {
	case errors.As(err, &ve):
		return errors.New(ve.Message)
	case errors.Is(err, auth.ErrLoginTaken):
		return errors.New("этот логин уже занят")
	}
	return err
}

// readPassword читает пароль: из терминала — без отображения и дважды,
// иначе — одну строку из stdin (для скриптов).
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !(errors.Is(err, io.EOF) && line != "") {
			return "", fmt.Errorf("чтение пароля из stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	fmt.Fprint(os.Stderr, "Пароль: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("чтение пароля: %w", err)
	}
	fmt.Fprint(os.Stderr, "Повторите пароль: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("чтение пароля: %w", err)
	}
	if string(first) != string(second) {
		return "", errors.New("пароли не совпадают")
	}
	return string(first), nil
}
