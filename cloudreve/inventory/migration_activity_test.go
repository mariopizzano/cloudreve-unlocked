package inventory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/ent/setting"
	"github.com/stretchr/testify/require"
)

// TestActivityMigration verifies that the additive activity table/settings
// migration is idempotent, so it can run on every startup after the upstream
// version marker has been written.
func TestActivityMigration(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "activity.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	require.NoError(t, migrateActivities(client, ctx))
	require.NoError(t, migrateActivitySettings(client, ctx))

	settingValue, err := client.Setting.Query().
		Where(setting.NameEQ("activity_log_enabled")).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "1", settingValue.Value)

	// Second run must not fail nor duplicate the setting.
	require.NoError(t, migrateActivities(client, ctx))
	require.NoError(t, migrateActivitySettings(client, ctx))
	count, err := client.Setting.Query().
		Where(setting.NameEQ("activity_log_enabled")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
