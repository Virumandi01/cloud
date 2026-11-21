package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"cloud-core/internal/config"
	"cloud-core/internal/users"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	cfg := config.Load()
	um, err := users.NewUserManager(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed loading user manager: %v", err)
	}

	switch os.Args[1] {
	case "set-passcode":
		if len(os.Args) < 4 {
			fmt.Println("Usage: admin-cli set-passcode <username> <new_passcode>")
			return
		}
		if err := um.SetUserPasscode(os.Args[2], os.Args[3]); err != nil {
			log.Fatalf("Error: %v", err)
		}
		fmt.Printf("Passcode updated for user '%s'\n", os.Args[2])

	case "set-quota":
		if len(os.Args) < 4 {
			fmt.Println("Usage: admin-cli set-quota <username> <limit_in_GB>")
			return
		}
		gb, err := strconv.ParseInt(os.Args[3], 10, 64)
		if err != nil {
			log.Fatalf("Invalid GB number: %v", err)
		}
		if err := um.SetUserQuota(os.Args[2], gb*1024*1024*1024); err != nil {
			log.Fatalf("Error: %v", err)
		}
		fmt.Printf("Quota updated to %d GB for user '%s'\n", gb, os.Args[2])

	case "create-user":
		if len(os.Args) < 5 {
			fmt.Println("Usage: admin-cli create-user <username> <passcode> <limit_in_GB>")
			return
		}
		gb, _ := strconv.ParseInt(os.Args[4], 10, 64)
		if err := um.CreateUser(os.Args[2], os.Args[3], gb*1024*1024*1024); err != nil {
			log.Fatalf("Error: %v", err)
		}
		fmt.Printf("User '%s' created with %d GB quota\n", os.Args[2], gb)

	case "list":
		for _, u := range um.ListAllUsers() {
			fmt.Printf("- %s (Role: %s, Used: %d B / Limit: %d B)\n", u.Username, u.Role, u.UsedBytes, u.StorageLimitBytes)
		}

	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Cloud Host Admin CLI:")
	fmt.Println("  admin-cli list")
	fmt.Println("  admin-cli create-user <username> <passcode> <limit_in_GB>")
	fmt.Println("  admin-cli set-passcode <username> <new_passcode>")
	fmt.Println("  admin-cli set-quota <username> <limit_in_GB>")
}
