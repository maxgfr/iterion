package forge

import (
	"fmt"

	"github.com/SocialGouv/iterion/pkg/secrets"
)

// SealOAuthAppSecret seals a forge OAuth app's client_secret, binding the
// sealed blob to the app id via AAD "forge_oauth_app:<id>" (same convention as
// forge_conn:<id> / generic_secret:<id>) so a sealed payload can't be silently
// transplanted to another app record.
func SealOAuthAppSecret(sealer secrets.Sealer, appID, clientSecret string) ([]byte, error) {
	if sealer == nil {
		return nil, fmt.Errorf("forge: nil sealer")
	}
	return sealer.Seal([]byte(clientSecret), forgeOAuthAppAAD(appID))
}

// OpenOAuthAppSecret returns an OAuth app's client_secret from its sealed blob.
func OpenOAuthAppSecret(sealer secrets.Sealer, appID string, sealed []byte) (string, error) {
	if sealer == nil {
		return "", fmt.Errorf("forge: nil sealer")
	}
	raw, err := sealer.Open(sealed, forgeOAuthAppAAD(appID))
	if err != nil {
		return "", fmt.Errorf("forge: open oauth app secret: %w", err)
	}
	return string(raw), nil
}

func forgeOAuthAppAAD(appID string) []byte {
	return []byte("forge_oauth_app:" + appID)
}
