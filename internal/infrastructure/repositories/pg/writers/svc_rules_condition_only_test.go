package writers

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	v1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func cleanupSvcRuleTestDB(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()

	_, err := pool.Exec(ctx, "SET session_replication_role = 'replica'")
	require.NoError(t, err)

	tables := []string{
		"sync_outbox",
		"service_rule_refs",
		"service_fqdn_rule_refs",
		"svc_svc_rules",
		"svc_fqdn_rules",
		"services",
		"k8s_metadata",
	}
	for _, tbl := range tables {
		_, err = pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", tbl))
		require.NoError(t, err)
	}

	_, err = pool.Exec(ctx, "SET session_replication_role = 'origin'")
	require.NoError(t, err)
}

func TestSvcSvcRuleConditionOnlyKeepsResourceVersionInSync(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	cleanupSvcRuleTestDB(t, pool)

	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	writer := NewWriter(&mockRegistry{pool: pool}, tx, ctx)

	rule := models.SvcSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "svc-ns",
				Name:      "svc-svc-rule-1",
			},
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "svc-from",
			},
			Namespace: "svc-ns",
		},
		ServiceToRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "svc-to",
			},
			Namespace: "svc-ns",
		},
		Action:      models.ActionAccept,
		Meta:        models.Meta{UID: uuid.NewString()},
		Description: "rv-sync-test",
	}

	err = writer.SyncSvcSvcRules(ctx, []models.SvcSvcRule{rule}, ports.EmptyScope{})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	var rvBefore int64
	err = pool.QueryRow(ctx, `
		SELECT resource_version FROM svc_svc_rules WHERE namespace = $1 AND name = $2`,
		rule.Namespace, rule.Name).Scan(&rvBefore)
	require.NoError(t, err)
	require.NotZero(t, rvBefore)

	rule.Meta.Conditions = []metav1.Condition{
		{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "Synced",
			Message: "Synced successfully",
		},
	}

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	writer2 := NewWriter(&mockRegistry{pool: pool}, tx2, ctx)

	err = writer2.SyncSvcSvcRules(ctx, []models.SvcSvcRule{rule}, ports.EmptyScope{}, ports.ConditionOnlyOperation{})
	require.NoError(t, err)
	require.NoError(t, tx2.Commit(ctx))

	var rvAfter int64
	var metaRV int64
	err = pool.QueryRow(ctx, `
		SELECT sr.resource_version
		FROM svc_svc_rules sr
		WHERE sr.namespace = $1 AND sr.name = $2`,
		rule.Namespace, rule.Name).Scan(&rvAfter)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		SELECT m.resource_version
		FROM k8s_metadata m
		JOIN svc_svc_rules sr ON sr.resource_version = m.resource_version
		WHERE sr.namespace = $1 AND sr.name = $2`,
		rule.Namespace, rule.Name).Scan(&metaRV)
	require.NoError(t, err)

	assert.NotEqual(t, rvBefore, rvAfter, "resource_version should bump after metadata update")
	assert.Equal(t, rvAfter, metaRV, "svc_svc_rules should track k8s_metadata resource_version")
}

func TestSvcFqdnRuleConditionOnlyKeepsResourceVersionInSync(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	cleanupSvcRuleTestDB(t, pool)

	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	writer := NewWriter(&mockRegistry{pool: pool}, tx, ctx)

	rule := models.SvcFqdnRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "svc-ns",
				Name:      "svc-fqdn-rule-1",
			},
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "svc-from",
			},
			Namespace: "svc-ns",
		},
		FQDN:        "example.com",
		Transport:   models.TCP,
		Action:      models.ActionAccept,
		Description: "rv-sync-test",
		Meta:        models.Meta{UID: uuid.NewString()},
	}

	err = writer.SyncSvcFqdnRules(ctx, []models.SvcFqdnRule{rule}, ports.EmptyScope{})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	var rvBefore int64
	err = pool.QueryRow(ctx, `
		SELECT resource_version FROM svc_fqdn_rules WHERE namespace = $1 AND name = $2`,
		rule.Namespace, rule.Name).Scan(&rvBefore)
	require.NoError(t, err)

	rule.Meta.Conditions = []metav1.Condition{
		{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "Synced",
			Message: "Synced successfully",
		},
	}

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	writer2 := NewWriter(&mockRegistry{pool: pool}, tx2, ctx)

	err = writer2.SyncSvcFqdnRules(ctx, []models.SvcFqdnRule{rule}, ports.EmptyScope{}, ports.ConditionOnlyOperation{})
	require.NoError(t, err)
	require.NoError(t, tx2.Commit(ctx))

	var rvAfter int64
	var metaRV int64
	err = pool.QueryRow(ctx, `
		SELECT resource_version FROM svc_fqdn_rules WHERE namespace = $1 AND name = $2`,
		rule.Namespace, rule.Name).Scan(&rvAfter)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		SELECT m.resource_version
		FROM k8s_metadata m
		JOIN svc_fqdn_rules fr ON fr.resource_version = m.resource_version
		WHERE fr.namespace = $1 AND fr.name = $2`,
		rule.Namespace, rule.Name).Scan(&metaRV)
	require.NoError(t, err)

	assert.NotEqual(t, rvBefore, rvAfter, "resource_version should bump after metadata update")
	assert.Equal(t, rvAfter, metaRV, "svc_fqdn_rules should track k8s_metadata resource_version")
}

func TestServiceDeleteCascadesSvcRules(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	cleanupSvcRuleTestDB(t, pool)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	writer := NewWriter(&mockRegistry{pool: pool}, tx, ctx)

	service := models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "svc-ns",
				Name:      "demo-service",
			},
		},
		Description: "cascade-test",
		Meta:        models.Meta{UID: uuid.NewString()},
	}
	service.Meta.TouchOnCreate()

	err = writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{})
	require.NoError(t, err)

	svcRule := models.SvcSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "svc-ns",
				Name:      "cascade-svcsvc",
			},
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       service.Name,
			},
			Namespace: service.Namespace,
		},
		ServiceToRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       "other-service",
			},
			Namespace: service.Namespace,
		},
		Action: models.ActionAccept,
		Meta:   models.Meta{UID: uuid.NewString()},
	}

	fqdnRule := models.SvcFqdnRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "svc-ns",
				Name:      "cascade-fqdn",
			},
		},
		ServiceFromRef: v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Service",
				Name:       service.Name,
			},
			Namespace: service.Namespace,
		},
		FQDN:      "cascade.example.com",
		Transport: models.TCP,
		Action:    models.ActionAccept,
		Meta:      models.Meta{UID: uuid.NewString()},
	}

	require.NoError(t, writer.SyncSvcSvcRules(ctx, []models.SvcSvcRule{svcRule}, ports.EmptyScope{}))
	require.NoError(t, writer.SyncSvcFqdnRules(ctx, []models.SvcFqdnRule{fqdnRule}, ports.EmptyScope{}))
	require.NoError(t, tx.Commit(ctx))

	var svcSvcRV int64
	var svcFqdnRV int64
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT resource_version FROM svc_svc_rules WHERE namespace = $1 AND name = $2`,
		svcRule.Namespace, svcRule.Name).Scan(&svcSvcRV))
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT resource_version FROM svc_fqdn_rules WHERE namespace = $1 AND name = $2`,
		fqdnRule.Namespace, fqdnRule.Name).Scan(&svcFqdnRV))

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	writer2 := NewWriter(&mockRegistry{pool: pool}, tx2, ctx)

	err = writer2.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpDelete))
	require.NoError(t, err)
	require.NoError(t, tx2.Commit(ctx))

	var count int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM svc_svc_rules WHERE namespace = $1 AND name = $2`,
		svcRule.Namespace, svcRule.Name).Scan(&count))
	assert.Equal(t, 0, count, "svc_svc_rules entry should be deleted with service")

	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM svc_fqdn_rules WHERE namespace = $1 AND name = $2`,
		fqdnRule.Namespace, fqdnRule.Name).Scan(&count))
	assert.Equal(t, 0, count, "svc_fqdn_rules entry should be deleted with service")

	var deletionTimestamp sql.NullTime
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT deletion_timestamp FROM k8s_metadata WHERE resource_version = $1`,
		svcSvcRV).Scan(&deletionTimestamp))
	assert.True(t, deletionTimestamp.Valid, "svc svc rule metadata should be marked as deleting")

	require.NoError(t, pool.QueryRow(ctx, `
		SELECT deletion_timestamp FROM k8s_metadata WHERE resource_version = $1`,
		svcFqdnRV).Scan(&deletionTimestamp))
	assert.True(t, deletionTimestamp.Valid, "svc fqdn rule metadata should be marked as deleting")

	var outboxCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sync_outbox
		WHERE resource_type = 'SvcSvcRule'
			AND resource_namespace = $1
			AND resource_name = $2
			AND operation = 'DELETE'`,
		svcRule.Namespace, svcRule.Name).Scan(&outboxCount))
	assert.Equal(t, 1, outboxCount, "svc svc rule delete should enqueue outbox record once")

	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sync_outbox
		WHERE resource_type = 'SvcFqdnRule'
			AND resource_namespace = $1
			AND resource_name = $2
			AND operation = 'DELETE'`,
		fqdnRule.Namespace, fqdnRule.Name).Scan(&outboxCount))
	assert.Equal(t, 1, outboxCount, "svc fqdn rule delete should enqueue outbox record once")
}
