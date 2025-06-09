package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pkg/errors"
)

type (
	// PortNumber -
	PortNumber = int32

	// PortRange -
	PortRange struct {
		pgtype.Range[PortNumber]
	}

	// PortMultirange -
	PortMultirange struct {
		pgtype.Multirange[PortRange]
	}

	// PortMultirangeArray -
	PortMultirangeArray []PortMultirange

	// TransportProtocol -
	TransportProtocol string

	// Traffic -
	Traffic string

	// Service -
	Service struct {
		Name         string `db:"name"`
		Namespace    string `db:"namespace"`
		Description  string `db:"description"`
		IngressPorts string `db:"ingress_ports"`
	}

	// AddressGroup -
	AddressGroup struct {
		Name        string   `db:"name"`
		Namespace   string   `db:"namespace"`
		Description string   `db:"description"`
		Addresses   []string `db:"addresses"`
	}

	// AddressGroupBinding -
	AddressGroupBinding struct {
		Name                  string `db:"name"`
		Namespace             string `db:"namespace"`
		ServiceName           string `db:"service_name"`
		ServiceNamespace      string `db:"service_namespace"`
		AddressGroupName      string `db:"address_group_name"`
		AddressGroupNamespace string `db:"address_group_namespace"`
	}

	// ServicePortsRef -
	ServicePortsRef struct {
		Name      string `db:"name"`
		Namespace string `db:"namespace"`
		Ports     string `db:"ports"`
	}

	// AddressGroupPortMapping -
	AddressGroupPortMapping struct {
		Name        string            `db:"name"`
		Namespace   string            `db:"namespace"`
		AccessPorts []ServicePortsRef `db:"access_ports"`
	}

	// RuleS2S -
	RuleS2S struct {
		Name                  string  `db:"name"`
		Namespace             string  `db:"namespace"`
		Traffic               Traffic `db:"traffic"`
		ServiceLocalName      string  `db:"service_local_name"`
		ServiceLocalNamespace string  `db:"service_local_namespace"`
		ServiceName           string  `db:"service_name"`
		ServiceNamespace      string  `db:"service_namespace"`
	}

	// SyncStatus -
	SyncStatus struct {
		UpdatedAt         time.Time `db:"updated_at"`
		TotalAffectedRows int64     `db:"total_affected_rows"`
	}
)

// Load -
func (s *SyncStatus) Load(ctx context.Context, c *pgx.Conn) error {
	const qry = `select updated_at, total_affected_rows from netguard.tbl_sync_status where id = (select max(id) from netguard.tbl_sync_status)`
	r, e := c.Query(ctx, qry)
	if e != nil {
		return e
	}
	*s, e = pgx.CollectOneRow(r, pgx.RowToStructByName[SyncStatus])
	return e
}

// Store -
func (s SyncStatus) Store(ctx context.Context, c *pgx.Conn) error {
	_, e := c.Exec(
		ctx,
		"insert into netguard.tbl_sync_status(total_affected_rows) values($1)",
		s.TotalAffectedRows)

	return e
}

// RegisterNetguardTypesOntoPGX -
func RegisterNetguardTypesOntoPGX(ctx context.Context, c *pgx.Conn) (err error) {
	defer func() {
		err = errors.WithMessage(err, "register 'netguard' types onto PGX")
	}()
	var pgType *pgtype.Type
	pgTypeMap := c.TypeMap()
	if pgType, err = c.LoadType(ctx, "netguard.port_ranges"); err != nil {
		return err
	}
	pgTypeMap.RegisterType(pgType)
	{
		var x PortMultirange
		pgTypeMap.RegisterDefaultPgType(x, pgType.Name)
		pgTypeMap.RegisterDefaultPgType(&x, pgType.Name)

		tn := pgType.Name + "_array"
		pgTypeMap.RegisterType(&pgtype.Type{
			Name:  tn,
			OID:   pgtype.Int4multirangeArrayOID,
			Codec: &pgtype.ArrayCodec{ElementType: pgType}},
		)
		var y PortMultirangeArray
		pgTypeMap.RegisterDefaultPgType(y, tn)
		pgTypeMap.RegisterDefaultPgType(&y, tn)
	}
	if pgType, err = c.LoadType(ctx, "netguard.transport_protocol"); err != nil {
		return err
	}
	pgTypeMap.RegisterType(pgType)
	{
		var x TransportProtocol
		pgTypeMap.RegisterDefaultPgType(x, pgType.Name)
		pgTypeMap.RegisterDefaultPgType(&x, pgType.Name)
	}
	if pgType, err = c.LoadType(ctx, "netguard.traffic"); err != nil {
		return err
	}
	pgTypeMap.RegisterType(pgType)
	{
		var x Traffic
		pgTypeMap.RegisterDefaultPgType(x, pgType.Name)
		pgTypeMap.RegisterDefaultPgType(&x, pgType.Name)
	}
	return nil
}
