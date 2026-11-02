package migration

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// verifyBaseline compares the existing schema with a rollback-only reference
// schema built from the selected embedded migration prefix. No application
// table/data is rewritten; the caller rolls back all reference DDL.
func (r *Runner) verifyBaseline(tx *sql.Tx, version uint) error {
	var actual string
	if err := tx.QueryRow(`SELECT current_schema()`).Scan(&actual); err != nil {
		return err
	}
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	reference := "blink_baseline_" + hex.EncodeToString(random)
	quoted := pgx.Identifier{reference}.Sanitize()
	if _, err := tx.Exec(`CREATE SCHEMA ` + quoted); err != nil {
		return fmt.Errorf("create temporary baseline comparison schema: %w", err)
	}
	if _, err := tx.Exec(`SET LOCAL search_path TO ` + quoted); err != nil {
		return err
	}
	entries, err := fs.ReadDir(r.files, ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		number, _, _ := strings.Cut(entry.Name(), "_")
		n, err := strconv.ParseUint(number, 10, 32)
		if err != nil {
			return err
		}
		if uint(n) > version {
			continue
		}
		body, err := fs.ReadFile(r.files, entry.Name())
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			return fmt.Errorf("build baseline reference migration %d: %w", n, err)
		}
	}
	expected, err := schemaSignature(tx, reference)
	if err != nil {
		return err
	}
	existing, err := schemaSignature(tx, actual)
	if err != nil {
		return err
	}
	if strings.Join(expected, "\n") != strings.Join(existing, "\n") {
		actualByObject := map[string]string{}
		for _, entry := range existing {
			parts := strings.SplitN(entry, "|", 4)
			actualByObject[strings.Join(parts[:3], "|")] = parts[3]
		}
		for _, entry := range expected {
			parts := strings.SplitN(entry, "|", 4)
			object := strings.Join(parts[:3], "|")
			if actualByObject[object] != parts[3] {
				return fmt.Errorf("baseline %d refused: schema mismatch at %s (expected %q, actual %q); history unchanged", version, object, parts[3], actualByObject[object])
			}
			delete(actualByObject, object)
		}
		for object := range actualByObject {
			return fmt.Errorf("baseline %d refused: unexpected schema object %s; history unchanged", version, object)
		}
	}
	return nil
}
func schemaSignature(tx *sql.Tx, schema string) ([]string, error) {
	rows, err := tx.Query(`
 SELECT 'column',table_name,column_name,
  concat_ws('|',ordinal_position::text,data_type,udt_name,COALESCE(character_maximum_length::text,''),is_nullable,COALESCE(column_default,''))
 FROM information_schema.columns WHERE table_schema=$1 AND table_name<>'schema_migrations'
 UNION ALL
 SELECT 'constraint',t.relname,c.conname,pg_get_constraintdef(c.oid,true)
 FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid JOIN pg_namespace n ON n.oid=t.relnamespace
 WHERE n.nspname=$1 AND t.relname<>'schema_migrations'
 UNION ALL
 SELECT 'index',t.relname,i.relname,pg_get_indexdef(i.oid)
 FROM pg_index x JOIN pg_class t ON t.oid=x.indrelid JOIN pg_class i ON i.oid=x.indexrelid JOIN pg_namespace n ON n.oid=t.relnamespace
 WHERE n.nspname=$1 AND t.relname<>'schema_migrations'
 `, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var signature []string
	for rows.Next() {
		var kind, table, name, definition string
		if err := rows.Scan(&kind, &table, &name, &definition); err != nil {
			return nil, err
		}
		definition = strings.ReplaceAll(definition, pgx.Identifier{schema}.Sanitize()+".", "")
		definition = strings.ReplaceAll(definition, schema+".", "")
		signature = append(signature, strings.Join([]string{kind, table, name, definition}, "|"))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(signature)
	return signature, nil
}
