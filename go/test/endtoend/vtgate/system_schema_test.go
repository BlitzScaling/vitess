/*
Copyright 2020 The Vitess Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vtgate

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"vitess.io/vitess/go/mysql"
	"vitess.io/vitess/go/test/endtoend/cluster"
)

// TestCheckConstraint test check constraints on CREATE TABLE
// This feature is supported from MySQL 8.0.16 and MariaDB 10.2.1.
func TestCheckConstraint(t *testing.T) {
	// Skipping as tests are run against MySQL 5.7
	t.Skip()

	conn, err := mysql.Connect(context.Background(), &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	query := `CREATE TABLE t7 (CHECK (c1 <> c2), c1 INT CHECK (c1 > 10), c2 INT CONSTRAINT c2_positive CHECK (c2 > 0), c3 INT CHECK (c3 < 100), CONSTRAINT c1_nonzero CHECK (c1 <> 0), CHECK (c1 > c3));`
	exec(t, conn, query)

	checkQuery := `SELECT CONSTRAINT_NAME FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS WHERE TABLE_NAME = 't7';`
	expected := `[[VARCHAR("t7_chk_1")] [VARCHAR("t7_chk_2")] [VARCHAR("c2_positive")] [VARCHAR("t7_chk_3")] [VARCHAR("c1_nonzero")] [VARCHAR("t7_chk_4")]]`

	assertMatches(t, conn, checkQuery, expected)

	cleanup := `DROP TABLE t7`
	exec(t, conn, cleanup)
}

func TestDbNameOverride(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.Nil(t, err)
	defer conn.Close()
	qr, err := conn.ExecuteFetch("SELECT distinct database() FROM information_schema.tables WHERE table_schema = database()", 1000, true)

	require.Nil(t, err)
	assert.Equal(t, 1, len(qr.Rows), "did not get enough rows back")
	assert.Equal(t, "vt_ks", qr.Rows[0][0].ToString())
}

func TestInformationSchemaQuery(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	assertSingleRowIsReturned(t, conn, "table_schema = 'ks'", "vt_ks")
	assertSingleRowIsReturned(t, conn, "table_schema = 'vt_ks'", "vt_ks")
	assertResultIsEmpty(t, conn, "table_schema = 'NONE'")
	assertSingleRowIsReturned(t, conn, "table_schema = 'performance_schema'", "performance_schema")
	assertResultIsEmpty(t, conn, "table_schema = 'PERFORMANCE_SCHEMA'")
	assertSingleRowIsReturned(t, conn, "table_schema = 'performance_schema' and table_name = 'users'", "performance_schema")
	assertResultIsEmpty(t, conn, "table_schema = 'performance_schema' and table_name = 'foo'")
	assertSingleRowIsReturned(t, conn, "table_schema = 'vt_ks' and table_name = 't1'", "vt_ks")
	assertSingleRowIsReturned(t, conn, "table_schema = 'ks' and table_name = 't1'", "vt_ks")
	// run end to end test for in statement.
	assertSingleRowIsReturned(t, conn, "table_schema IN ('ks')", "vt_ks")
	assertSingleRowIsReturned(t, conn, "table_schema IN ('vt_ks')", "vt_ks")
	assertSingleRowIsReturned(t, conn, "table_schema IN ('ks') and table_name = 't1'", "vt_ks")
	// run end to end test for and expression.
	assertSingleRowIsReturned(t, conn, "table_schema IN ('ks') and table_schema = 'ks'", "vt_ks")
	assertSingleRowIsReturned(t, conn, "table_schema IN ('ks') and table_schema = 'vt_ks'", "vt_ks")
	assertSingleRowIsReturned(t, conn, "table_schema IN ('vt_ks') and table_schema = 'ks'", "vt_ks")

	// TODO (ruimins) when trying to query a non-default keyspace in the following format, the router cannot tell the
	// equivalence of the keyspace name and its full database name, resulting in the query being routed to the same
	// vttablet twice and duplicate results
	// e.g. (example/demo) table_schema = 'vt_product_0' and table_name = 'product'
	// returns `(vt_product_0.product, vt_product_0.product)`

	// TODO (ruimins) when querying a routed table in a sharded keyspace, specifying the keyspace name will lead to only
	// a subset of results is returned
	// e.g. (example/demp) table_schema = 'customer' and table_name = 'customer'
	// returns `(vt_customer_80-.customer)`
	// e.g. (example/demp) table_name = 'customer'
	// returns `(vt_customer_80-.customer, vt_customer_-80.customer)`
}

func assertResultIsEmpty(t *testing.T, conn *mysql.Conn, pre string) {
	t.Run(pre, func(t *testing.T) {
		qr, err := conn.ExecuteFetch("SELECT distinct table_schema FROM information_schema.tables WHERE "+pre, 1000, true)
		require.NoError(t, err)
		assert.Empty(t, qr.Rows)
	})
}

func assertSingleRowIsReturned(t *testing.T, conn *mysql.Conn, predicate string, expectedKs string) {
	t.Run(predicate, func(t *testing.T) {
		qr, err := conn.ExecuteFetch("SELECT distinct table_schema FROM information_schema.tables WHERE "+predicate, 1000, true)
		require.NoError(t, err)
		assert.Equal(t, 1, len(qr.Rows), "did not get enough rows back")
		assert.Equal(t, expectedKs, qr.Rows[0][0].ToString())
	})
}

func TestInformationSchemaWithSubquery(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	result := exec(t, conn, "SELECT column_name FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = (SELECT SCHEMA()) AND TABLE_NAME = 'not_exists'")
	assert.Empty(t, result.Rows)
}

func TestInformationSchemaQueryGetsRoutedToTheRightTableAndKeyspace(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	_ = exec(t, conn, "SELECT * FROM t1000") // test that the routed table is available to us
	result := exec(t, conn, "SELECT * FROM information_schema.tables WHERE table_schema = database() and table_name='t1000'")
	assert.NotEmpty(t, result.Rows)
}

func TestFKConstraintUsingInformationSchema(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	query := "select fk.referenced_table_name as to_table, fk.referenced_column_name as primary_key, fk.column_name as `column`, fk.constraint_name as name, rc.update_rule as on_update, rc.delete_rule as on_delete from information_schema.referential_constraints as rc join information_schema.key_column_usage as fk using (constraint_schema, constraint_name) where fk.referenced_column_name is not null and fk.table_schema = database() and fk.table_name = 't7_fk' and rc.constraint_schema = database() and rc.table_name = 't7_fk'"
	assertMatches(t, conn, query, `[[VARCHAR("t7_xxhash") VARCHAR("uid") VARCHAR("t7_uid") VARCHAR("t7_fk_ibfk_1") VARCHAR("CASCADE") VARCHAR("SET NULL")]]`)
}

func TestConnectWithSystemSchema(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	for _, dbname := range []string{"information_schema", "mysql", "performance_schema", "sys"} {
		connParams := vtParams
		connParams.DbName = dbname
		conn, err := mysql.Connect(ctx, &connParams)
		require.NoError(t, err)
		exec(t, conn, `select @@max_allowed_packet from dual`)
		conn.Close()
	}
}

func TestUseSystemSchema(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()
	for _, dbname := range []string{"information_schema", "mysql", "performance_schema", "sys"} {
		exec(t, conn, fmt.Sprintf("use %s", dbname))
		exec(t, conn, `select @@max_allowed_packet from dual`)
	}
}

func TestSystemSchemaQueryWithoutQualifier(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	queryWithQualifier := fmt.Sprintf("select t.table_schema,t.table_name,c.column_name,c.column_type "+
		"from information_schema.tables t "+
		"join information_schema.columns c "+
		"on c.table_schema = t.table_schema and c.table_name = t.table_name "+
		"where t.table_schema = '%s' and c.table_schema = '%s'", KeyspaceName, KeyspaceName)
	qr1 := exec(t, conn, queryWithQualifier)

	queryWithoutQualifier := fmt.Sprintf("select t.table_schema,t.table_name,c.column_name,c.column_type "+
		"from tables t "+
		"join columns c "+
		"on c.table_schema = t.table_schema and c.table_name = t.table_name "+
		"where t.table_schema = '%s' and c.table_schema = '%s'", KeyspaceName, KeyspaceName)
	exec(t, conn, "use information_schema")
	qr2 := exec(t, conn, queryWithoutQualifier)
	require.Equal(t, qr1, qr2)

	connParams := vtParams
	connParams.DbName = "information_schema"
	conn2, err := mysql.Connect(ctx, &connParams)
	require.NoError(t, err)
	defer conn2.Close()

	qr3 := exec(t, conn2, queryWithoutQualifier)
	require.Equal(t, qr2, qr3)
}

func TestMultipleSchemaPredicates(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	query := fmt.Sprintf("select t.table_schema,t.table_name,c.column_name,c.column_type "+
		"from information_schema.tables t "+
		"join information_schema.columns c "+
		"on c.table_schema = t.table_schema and c.table_name = t.table_name "+
		"where t.table_schema = '%s' and c.table_schema = '%s' and c.table_schema = '%s' and c.table_schema = '%s'", KeyspaceName, KeyspaceName, KeyspaceName, KeyspaceName)
	qr1 := exec(t, conn, query)
	require.EqualValues(t, 4, len(qr1.Fields))

	// test a query with two keyspace names
	query = fmt.Sprintf("select t.table_schema,t.table_name,c.column_name,c.column_type "+
		"from information_schema.tables t "+
		"join information_schema.columns c "+
		"on c.table_schema = t.table_schema and c.table_name = t.table_name "+
		"where t.table_schema = '%s' and c.table_schema = '%s' and c.table_schema = '%s'", KeyspaceName, KeyspaceName, "a")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 4, len(qr1.Fields))
	require.EqualValues(t, 0, len(qr1.Rows))

	// test IN statement with two keyspace names
	query = fmt.Sprintf("select table_schema,table_name "+
		"from information_schema.tables "+
		"where table_schema in ('%s', '%s')", KeyspaceName, "a")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 2, len(qr1.Fields))
	require.EqualValues(t, 17, len(qr1.Rows))

	// test AND expression with two keyspace names
	query = fmt.Sprintf("select table_schema "+
		"from information_schema.tables "+
		"where table_schema = '%s' and table_schema = '%s'", KeyspaceName, "a")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 1, len(qr1.Fields))
	require.EqualValues(t, 0, len(qr1.Rows))

	// test OR expression with two keyspace names
	query = fmt.Sprintf("select table_schema "+
		"from information_schema.tables "+
		"where table_schema = '%s' or table_schema = '%s'", KeyspaceName, "a")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 1, len(qr1.Fields))
	require.EqualValues(t, 17, len(qr1.Rows))
}

func TestMultipleSchemaPredicatesDuplicatesHandling(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	// test OR with invalid schema
	query := fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where table_schema = '%s' or (table_schema = '%s' and table_name = '%s')", "a", KeyspaceName, "t1")
	qr1 := exec(t, conn, query)
	require.EqualValues(t, 1, len(qr1.Rows))

	// test OR with invalid subexpression
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where table_schema = '%s' and table_schema = '%s' or table_name = '%s'", "a", KeyspaceName, "t1")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 1, len(qr1.Rows))

	// test OR with same predicates
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where (table_schema = '%s' and table_name = '%s') or (table_schema = '%s' and table_name = '%s')", KeyspaceName, "t1", KeyspaceName, "t1")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 1, len(qr1.Rows))

	//test OR with different table_name predicates
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where (table_schema = '%s' and table_name = '%s') or (table_schema = '%s' and table_name = '%s')", KeyspaceName, "t1", KeyspaceName, "t2")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 2, len(qr1.Rows))

	// test no duplicated results
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where table_schema = '%s' or (table_schema = '%s' and table_name = '%s')", KeyspaceName, KeyspaceName, "t1")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 17, len(qr1.Rows))

	// test no duplicated results
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where table_name = '%s' or (table_schema = '%s' and table_name = '%s')", "t2", KeyspaceName, "t1")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 2, len(qr1.Rows))

	// test no duplicated results
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where table_name = '%s' or (table_schema = '%s' and table_name = '%s')", "t1", KeyspaceName, "t1")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 1, len(qr1.Rows))

	// test with IN statement
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where (table_schema, table_name) in (('%s','%s'), ('%s','%s'))", KeyspaceName, "t1", KeyspaceName, "t2")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 2, len(qr1.Rows))

	// test mixed AND/OR with IN statement
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where table_schema = '%s' and table_name in ('%s','%s')", KeyspaceName, "t1", "t2")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 2, len(qr1.Rows))

	// test multiple tables in same keyspace
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where table_name in ('%s','%s')", "t1", "t2")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 2, len(qr1.Rows))

	// test invalid table_name
	query = fmt.Sprintf("select table_schema,table_name from information_schema.tables "+
		"where table_schema = '%s' and table_name in ('%s', '%s', '%s')", KeyspaceName, "t1", "tinvalid1", "tinvalid2")
	qr1 = exec(t, conn, query)
	require.EqualValues(t, 1, len(qr1.Rows))
}

func TestSystemSchemaQueryWithUnion(t *testing.T) {
	defer cluster.PanicHandler(t)
	ctx := context.Background()
	conn, err := mysql.Connect(ctx, &vtParams)
	require.NoError(t, err)
	defer conn.Close()

	testcases := []struct {
		predicates  []string
		expectedKSs []string
	}{{
		predicates:  []string{"table_schema = 'ks'", "table_schema = 'performance_schema'"},
		expectedKSs: []string{"vt_ks", "performance_schema"},
	}, {
		predicates:  []string{"table_schema in ('ks')", "table_schema in ('performance_schema')"},
		expectedKSs: []string{"vt_ks", "performance_schema"},
	}}
	for _, tc := range testcases {
		assertUnionPredicatesResults(t, conn, tc.predicates, tc.expectedKSs, len(tc.expectedKSs))
	}
}

func assertUnionPredicatesResults(t *testing.T, conn *mysql.Conn, predicates []string, expectedKSs []string, expectedRows int) {
	t.Run("", func(t *testing.T) {
		var sql string
		for i, predicate := range predicates {
			if i != 0 {
				sql = sql + " union all "
			}
			sql = sql + "SELECT distinct table_schema FROM information_schema.tables WHERE " + predicate
		}
		qr, err := conn.ExecuteFetch(sql, 1000, true)
		require.NoError(t, err)
		assert.Equal(t, expectedRows, len(qr.Rows), "did not get matched rows back")
		for i, expectedKs := range expectedKSs {
			assert.Equal(t, expectedKs, qr.Rows[i][0].ToString())
		}
	})
}
