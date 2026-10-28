package main

import (
	"log"
	"net/http"
)

func main() {
	log.Println("🚀 Starting ShizukuBot Plugin Market Server...")

	if err := InitConfig(); err != nil {
		log.Fatalf("❌ Failed to init config: %v", err)
	}

	if err := InitAuth(); err != nil {
		log.Printf("⚠️ Auth init warning: %v", err)
	}

	if err := InitStorage(); err != nil {
		log.Fatalf("❌ Failed to init storage: %v", err)
	}

	r := SetupRouter()

	log.Printf("✅ Server running on %s", config.Server.Addr)
	log.Fatal(http.ListenAndServe(config.Server.Addr, r))
}
