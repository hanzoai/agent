package application

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/hanzoai/agents/control-plane/internal/cli/framework"
	"github.com/hanzoai/agents/control-plane/internal/config"
	"github.com/hanzoai/agents/control-plane/internal/core/services"
	"github.com/hanzoai/agents/control-plane/internal/infrastructure/process"
	"github.com/hanzoai/agents/control-plane/internal/infrastructure/storage"
	"github.com/hanzoai/agents/control-plane/internal/logger"
	didServices "github.com/hanzoai/agents/control-plane/internal/services"
	storageInterface "github.com/hanzoai/agents/control-plane/internal/storage"
)

// CreateServiceContainer creates and wires up all services for the CLI commands
func CreateServiceContainer(cfg *config.Config, hanzo-agentsHome string) *framework.ServiceContainer {
	// Create infrastructure components
	fileSystem := storage.NewFileSystemAdapter()
	registryPath := filepath.Join(hanzo-agentsHome, "installed.json")
	registryStorage := storage.NewLocalRegistryStorage(fileSystem, registryPath)
	processManager := process.NewProcessManager()
	portManager := process.NewPortManager()

	// Create storage provider based on configuration
	storageFactory := &storageInterface.StorageFactory{}
	storageProvider, _, err := storageFactory.CreateStorage(cfg.Storage)
	if err != nil {
		// Log error - database storage initialization failed
		// In production, this should be handled more gracefully
		storageProvider = nil
	}

	// Create services
	packageService := services.NewPackageService(registryStorage, fileSystem, hanzo-agentsHome)
	agentService := services.NewAgentService(processManager, portManager, registryStorage, nil, hanzo-agentsHome) // nil agentClient for now
	devService := services.NewDevService(processManager, portManager, fileSystem)

	// Create DID services if enabled
	var didService *didServices.DIDService
	var vcService *didServices.VCService
	var keystoreService *didServices.KeystoreService
	var didRegistry *didServices.DIDRegistry

	if cfg.Features.DID.Enabled {
		// Create keystore service
		keystoreService, err = didServices.NewKeystoreService(&cfg.Features.DID.Keystore)
		if err != nil {
			// Log error but continue - DID system will be disabled
			keystoreService = nil
		}

		// Create DID registry with database storage (required)
		if storageProvider != nil {
			didRegistry = didServices.NewDIDRegistryWithStorage(storageProvider)
		} else {
			// DID registry requires database storage, skip if not available
			didRegistry = nil
		}

		if didRegistry != nil {
			if err := didRegistry.Initialize(); err != nil {
				// Log error but continue
				didRegistry = nil
			}
		}

		// Create DID service
		if keystoreService != nil && didRegistry != nil {
			didService = didServices.NewDIDService(&cfg.Features.DID, keystoreService, didRegistry)

			// Generate af server ID based on hanzo-agents home directory
			// This ensures each hanzo-agents instance has a unique ID while being deterministic
			hanzo-agentsServerID := generateHanzoAgentsServerID(hanzo-agentsHome)
			if err := didService.Initialize(hanzo-agentsServerID); err != nil {
				logger.Logger.Warn().Err(err).Msg("failed to initialize DID service")
				didService = nil
			} else {
				// Create VC service with database storage (required)
				if storageProvider != nil {
					vcService = didServices.NewVCService(&cfg.Features.DID, didService, storageProvider)
				}

				if vcService != nil {
					if err := vcService.Initialize(); err != nil {
						logger.Logger.Warn().Err(err).Msg("failed to initialize VC service")
						vcService = nil
					}
				}
			}
		}
	}

	return &framework.ServiceContainer{
		PackageService:  packageService,
		AgentService:    agentService,
		DevService:      devService,
		DIDService:      didService,
		VCService:       vcService,
		KeystoreService: keystoreService,
		DIDRegistry:     didRegistry,
		StorageProvider: storageProvider,
	}
}

// CreateServiceContainerWithDefaults creates a service container with default configuration
func CreateServiceContainerWithDefaults(hanzo-agentsHome string) *framework.ServiceContainer {
	// Use default config for now
	cfg := &config.Config{} // This will be enhanced when config is properly structured
	return CreateServiceContainer(cfg, hanzo-agentsHome)
}

// generateHanzoAgentsServerID creates a deterministic af server ID based on the hanzo-agents home directory.
// This ensures each hanzo-agents instance has a unique ID while being deterministic for the same installation.
func generateHanzoAgentsServerID(hanzo-agentsHome string) string {
	// Use the absolute path of hanzo-agents home to generate a deterministic ID
	absPath, err := filepath.Abs(hanzo-agentsHome)
	if err != nil {
		// Fallback to the original path if absolute path fails
		absPath = hanzo-agentsHome
	}

	// Create a hash of the hanzo-agents home path to generate a unique but deterministic ID
	hash := sha256.Sum256([]byte(absPath))

	// Use first 16 characters of the hex hash as the af server ID
	// This provides uniqueness while keeping the ID manageable
	hanzo-agentsServerID := hex.EncodeToString(hash[:])[:16]

	return hanzo-agentsServerID
}
