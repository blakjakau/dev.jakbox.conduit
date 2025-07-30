package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const appName = "conduit" // Used for config directory
const apiKeyFileName = "api-key"
const apiKeyLength = 32 // 32 bytes of entropy -> 44 chars Base64 (approx)

// manageAPIKey handles the --key flag: generates, stores, and prints the API key.
// If `forceGenerateAndPrint` is true, it will ensure a key exists, print it, and set it.
// If `forceGenerateAndPrint` is false, it will only attempt to load an existing key.
func manageAPIKey(forceGenerateAndPrint bool) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("Error getting user config directory: %v", err)
	}
	
	appConfigDir := filepath.Join(configDir, appName)
	// Ensure the application's config directory exists
	if err := os.MkdirAll(appConfigDir, 0700); err != nil {
		log.Fatalf("Error creating application config directory %s: %v", appConfigDir, err)
	}
	
	apiKeyPath := filepath.Join(appConfigDir, apiKeyFileName)

	// Try to read existing key
	content, err := ioutil.ReadFile(apiKeyPath)
	if err == nil {
		requiredAPIKey = strings.TrimSpace(string(content))
		fmt.Printf("Existing API Key found (%s)\n", apiKeyPath)
		if forceGenerateAndPrint {
			// If we successfully read an existing key AND --key was passed,
			// we're done; the key is now in `requiredAPIKey` and has been printed.
			return
		}
		// If forceGenerateAndPrint is false (i.e., normal server startup),
		// we've loaded the key, so we're good to go.
		return
	}

	// If the file does not exist.
	if os.IsNotExist(err) && forceGenerateAndPrint {
		key, err := generateAPIKey()
		if err != nil {
			log.Fatalf("Error generating API key: %v", err)
		}

		// Write with restricted permissions (read/write for owner only)
		err = ioutil.WriteFile(apiKeyPath, []byte(key), 0600) // Ensure permissions are restrictive
		if err != nil {
			log.Fatalf("Error writing API key to %s: %v", apiKeyPath, err)
		}

		requiredAPIKey = key
		fmt.Printf("Generated new API Key (saved to %s):\n%s\n", apiKeyPath, requiredAPIKey)
		return
	} else if os.IsNotExist(err) && !forceGenerateAndPrint {
		// If no key file exists and --key was NOT passed (normal server startup),
		// proceed without requiring an API key. `requiredAPIKey` remains "".
		log.Printf("No API key file found at %s. Running without an API key requirement for no-origin requests.", apiKeyPath)
		return
	}

	// Other error reading file
	log.Fatalf("Error reading API key from %s: %v", apiKeyPath, err)
}

// generateAPIKey creates a cryptographically secure random Base64 string.
func generateAPIKey() (string, error) {
	bytes := make([]byte, apiKeyLength)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
