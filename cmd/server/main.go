package main

import (
	"fmt"
	"log"

	"github.com/ChenXuan-github/rock-paper-scissors/internal/api/router"
	"github.com/ChenXuan-github/rock-paper-scissors/internal/config"
)

func main() {
	cfg, err := config.Load("configs/application.yml")
	if err != nil {
		log.Fatal(err)
	}

	r := router.New()
	address := fmt.Sprintf(":%d", cfg.Server.Port)

	if err := r.Run(address); err != nil {
		log.Fatal(err)
	}
}
