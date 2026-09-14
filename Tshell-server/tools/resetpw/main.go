package main

import (
	"fmt"

	"BackendTemplate/pkg/database"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	database.DatabaseName = "./database.db"
	database.ConnectDateBase()
	h, err := bcrypt.GenerateFromPassword([]byte("admin"), 10)
	if err != nil {
		panic(err)
	}
	_, err = database.Engine.Exec("UPDATE users SET password_hash=?, login_attempts=0, locked=0 WHERE username='admin'", string(h))
	if err != nil {
		panic(err)
	}
	fmt.Println("admin password reset to 'admin'")
}
