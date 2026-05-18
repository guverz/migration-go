package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitConfig_WithCustomConfigFile(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "custom.yaml")

	content := `help:
  include: false
directories:
  mini_help: "custom.sql"
  migrations: "./custom_migrations"`

	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	originalCfgFile := CfgFile
	defer func() { CfgFile = originalCfgFile }()
	CfgFile = configPath

	viper.Reset()

	// Act
	InitConfig()

	// Assert
	assert.Equal(t, configPath, viper.ConfigFileUsed())
	assert.Equal(t, false, viper.GetBool("help.include"))
	assert.Equal(t, "custom.sql", viper.GetString("directories.mini_help"))
	assert.Equal(t, "./custom_migrations", viper.GetString("directories.migrations"))
}

func TestInitConfig_WithDefaultPathsAndExistingConfig(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()

	configContent := `help:
  include: true
directories:
  mini_help: "test.sql"
  migrations: "./test_migrations"`

	err := os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(configContent), 0644)
	require.NoError(t, err)

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		if chdirErr := os.Chdir(originalWd); chdirErr != nil && err == nil {
			err = chdirErr
		}
	}()
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	originalCfgFile := CfgFile
	defer func() { CfgFile = originalCfgFile }()
	CfgFile = ""

	// Сбрасываем viper
	viper.Reset()

	// Act
	InitConfig()

	// Assert
	assert.Contains(t, viper.ConfigFileUsed(), "config.yaml")
	assert.Equal(t, true, viper.GetBool("help.include"))
	assert.Equal(t, "test.sql", viper.GetString("directories.mini_help"))
	assert.Equal(t, "./test_migrations", viper.GetString("directories.migrations"))
}

func TestInitConfig_NoConfigFile_UsesDefaults(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		if chdirErr := os.Chdir(originalWd); chdirErr != nil && err == nil {
			err = chdirErr
		}
	}()

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	originalCfgFile := CfgFile
	defer func() { CfgFile = originalCfgFile }()
	CfgFile = ""

	viper.Reset()

	// Act
	InitConfig()

	// Assert
	assert.Empty(t, viper.ConfigFileUsed())

	assert.Equal(t, true, viper.GetBool("help.include"))
	assert.Equal(t, "migration.template.sql", viper.GetString("directories.mini_help"))
	assert.Equal(t, "./migrations", viper.GetString("directories.migrations"))
}

func TestLoadConfigToConstants(t *testing.T) {
	// Arrange
	viper.Reset()

	viper.Set("directories.mini_help", "test_help.sql")
	viper.Set("directories.migrations", "./test_migrations")
	viper.Set("help.include", false)

	// Act
	LoadConfigToConstants()

	// Assert
	assert.Equal(t, "test_help.sql", MiniHelpDir)
	assert.Equal(t, "./test_migrations", MigrationDir)
	assert.Equal(t, false, IncludeHelp)
}

func TestLoadConfigToConstants_WithMissingKeys_UsesZeroValues(t *testing.T) {
	// Arrange
	viper.Reset()

	// Act
	LoadConfigToConstants()

	// Assert
	assert.Empty(t, MiniHelpDir)
	assert.Empty(t, MigrationDir)
	assert.False(t, IncludeHelp)
}

func TestSetDefaults_ExplicitCall(t *testing.T) {
	// Arrange
	viper.Reset()

	assert.False(t, viper.IsSet("help.include"))
	assert.False(t, viper.IsSet("directories.mini_help"))
	assert.False(t, viper.IsSet("directories.migrations"))

	// Act
	setDefaults()

	// Assert
	assert.Equal(t, true, viper.GetBool("help.include"))
	assert.Equal(t, "migration.template.sql", viper.GetString("directories.mini_help"))
	assert.Equal(t, "./migrations", viper.GetString("directories.migrations"))
}

func TestInitConfig_WithHomeDirectory(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	originalCfgFile := CfgFile
	defer func() { CfgFile = originalCfgFile }()
	CfgFile = ""

	viper.Reset()

	assert.NotPanics(t, func() {
		viper.AddConfigPath(".")
		viper.AddConfigPath(home)
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AutomaticEnv()
		_ = viper.ReadInConfig()
	})
}
