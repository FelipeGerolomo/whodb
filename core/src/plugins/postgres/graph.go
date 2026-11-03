// Copyright 2025 Clidey, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package postgres

import (
	"gorm.io/gorm"

	"github.com/clidey/whodb/core/src/engine"
	"github.com/clidey/whodb/core/src/plugins"
	"github.com/clidey/whodb/core/src/source"
)

const graphQuery = `
WITH fk_constraints AS (
    SELECT DISTINCT
        ccu.table_name AS table1,
        tc.table_name AS table2,
        'OneToMany' AS relation,
        kcu.column_name AS source_column,
        ccu.column_name AS target_column
    FROM
        information_schema.table_constraints AS tc
    JOIN
        information_schema.key_column_usage AS kcu
    ON
        tc.constraint_name = kcu.constraint_name
    JOIN
        information_schema.constraint_column_usage AS ccu
    ON
        ccu.constraint_name = tc.constraint_name
    WHERE
        tc.constraint_type = 'FOREIGN KEY'
        AND tc.table_schema = ?
        AND ccu.table_schema = ?
),
pk_constraints AS (
    SELECT DISTINCT
        tc.table_name AS table1,
        ccu.table_name AS table2,
        'OneToOne' AS relation,
        NULL::text AS source_column,
        NULL::text AS target_column
    FROM
        information_schema.table_constraints AS tc
    JOIN
        information_schema.key_column_usage AS kcu
    ON
        tc.constraint_name = kcu.constraint_name
    JOIN
        information_schema.constraint_column_usage AS ccu
    ON
        ccu.constraint_name = tc.constraint_name
    WHERE
        tc.constraint_type = 'PRIMARY KEY'
        AND tc.table_schema = ?
        AND ccu.table_schema = ?
        AND tc.table_name != ccu.table_name
),
unique_constraints AS (
    SELECT DISTINCT
        tc.table_name AS table1,
        ccu.table_name AS table2,
        'ManyToOne' AS relation,
        NULL::text AS source_column,
        NULL::text AS target_column
    FROM
        information_schema.table_constraints AS tc
    JOIN
        information_schema.key_column_usage AS kcu
    ON
        tc.constraint_name = kcu.constraint_name
    JOIN
        information_schema.constraint_column_usage AS ccu
    ON
        ccu.constraint_name = tc.constraint_name
    WHERE
        tc.constraint_type = 'UNIQUE'
        AND tc.table_schema = ?
        AND ccu.table_schema = ?
        AND tc.table_name != ccu.table_name
),
many_to_many_constraints AS (
    SELECT DISTINCT
        kcu1.table_name AS table1,
        kcu2.table_name AS table2,
        'ManyToMany' AS relation,
        NULL::text AS source_column,
        NULL::text AS target_column
    FROM
        information_schema.key_column_usage kcu1
    JOIN
        information_schema.referential_constraints rc
    ON
        kcu1.constraint_name = rc.constraint_name
    JOIN
        information_schema.key_column_usage kcu2
    ON
        kcu2.constraint_name = rc.unique_constraint_name
    WHERE
        kcu1.ordinal_position = 1 AND kcu2.ordinal_position = 2
        AND kcu1.table_schema = ?
        AND kcu2.table_schema = ?
)
SELECT * FROM fk_constraints
UNION
SELECT * FROM pk_constraints
UNION
SELECT * FROM unique_constraints
UNION
SELECT * FROM many_to_many_constraints
`

func (p *PostgresPlugin) GetGraphQueryDB(db *gorm.DB, schema string) *gorm.DB {
	return db.Raw(graphQuery, schema, schema, schema, schema, schema, schema, schema, schema)
}

// The schemas of the all-schemas graph: those the user can use, without the system ones.
const allSchemasQuery = `
SELECT nspname
FROM pg_namespace
WHERE has_schema_privilege(oid, 'USAGE')
    AND nspname NOT IN ('pg_catalog', 'information_schema')
    AND nspname NOT LIKE 'pg\_%'
ORDER BY nspname
`

// The foreign keys between the tables of those schemas, across schemas too, one row per column
// pair, named as in graphQuery's fk_constraints.
const allSchemasGraphQuery = `
SELECT DISTINCT
    target_ns.nspname || '.' || target.relname AS table1,
    source_ns.nspname || '.' || source.relname AS table2,
    'OneToMany' AS relation,
    source_column.attname AS source_column,
    target_column.attname AS target_column
FROM pg_constraint con
JOIN pg_class source ON source.oid = con.conrelid
JOIN pg_namespace source_ns ON source_ns.oid = source.relnamespace
JOIN pg_class target ON target.oid = con.confrelid
JOIN pg_namespace target_ns ON target_ns.oid = target.relnamespace
CROSS JOIN LATERAL unnest(con.conkey, con.confkey) AS columns(source_attnum, target_attnum)
JOIN pg_attribute source_column ON source_column.attrelid = con.conrelid AND source_column.attnum = columns.source_attnum
JOIN pg_attribute target_column ON target_column.attrelid = con.confrelid AND target_column.attnum = columns.target_attnum
WHERE con.contype = 'f'
    AND source_ns.nspname IN ?
    AND target_ns.nspname IN ?
`

type allSchemasRelation struct {
	Table1       string
	Table2       string
	Relation     string
	SourceColumn *string
	TargetColumn *string
}

// GetGraph returns the graph of one schema, as every gorm plugin does, or of every schema when
// schema is source.AllSchemasGraphScope.
func (p *PostgresPlugin) GetGraph(config *engine.PluginConfig, schema string) ([]engine.GraphUnit, error) {
	if schema != source.AllSchemasGraphScope {
		return p.GormPlugin.GetGraph(config, schema)
	}

	var schemas []string
	var relations []allSchemasRelation
	_, err := plugins.WithConnection(config, p.DB, func(db *gorm.DB) (struct{}, error) {
		if err := db.Raw(allSchemasQuery).Scan(&schemas).Error; err != nil {
			return struct{}{}, err
		}
		if len(schemas) == 0 {
			return struct{}{}, nil
		}
		return struct{}{}, db.Raw(allSchemasGraphQuery, schemas, schemas).Scan(&relations).Error
	})
	if err != nil {
		return nil, err
	}

	relationsByTable := map[string][]engine.GraphUnitRelationship{}
	for _, relation := range relations {
		relationsByTable[relation.Table1] = append(relationsByTable[relation.Table1], engine.GraphUnitRelationship{
			Name:             relation.Table2,
			RelationshipType: engine.GraphUnitRelationshipType(relation.Relation),
			SourceColumn:     relation.SourceColumn,
			TargetColumn:     relation.TargetColumn,
		})
	}

	var units []engine.GraphUnit
	for _, schemaName := range schemas {
		storageUnits, err := p.GetStorageUnits(config, schemaName)
		if err != nil {
			return nil, err
		}
		for _, storageUnit := range storageUnits {
			storageUnit.Name = schemaName + "." + storageUnit.Name
			units = append(units, engine.GraphUnit{Unit: storageUnit, Relations: relationsByTable[storageUnit.Name]})
		}
	}
	return units, nil
}
