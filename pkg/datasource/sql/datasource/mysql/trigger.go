/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mysql

import (
	"context"
	"database/sql"
	"strings"

	"github.com/pkg/errors"
	sqlUtil "github.com/seata/seata-go/pkg/common/sql"
	"github.com/seata/seata-go/pkg/constant"
	"github.com/seata/seata-go/pkg/datasource/sql/types"
)

type mysqlTrigger struct {
}

func NewMysqlTrigger() *mysqlTrigger {
	return &mysqlTrigger{}
}

// LoadOne
func (m *mysqlTrigger) LoadOne(ctx context.Context, dbName string, tableName string, conn *sql.Conn) (types.TableMeta, error) {
	tableMeta := types.TableMeta{Name: tableName,
		Columns: make(map[string]types.ColumnMeta),
		Indexs:  make(map[string]types.IndexMeta),
	}

	colMetas, err := m.getColumns(ctx, dbName, tableName, conn)
	if err != nil {
		return types.TableMeta{}, errors.Wrapf(err, "Could not found any column in the table: %s", tableName)
	}

	columns := make([]string, 0)
	for _, column := range colMetas {
		tableMeta.Columns[column.ColumnName] = column
		columns = append(columns, column.ColumnName)
	}
	tableMeta.ColumnNames = columns

	indexes, err := m.getIndexes(ctx, dbName, tableName, conn)
	if err != nil {
		return types.TableMeta{}, errors.Wrapf(err, "Could not found any index in the table: %s", tableName)
	}
	for _, index := range indexes {
		col := tableMeta.Columns[index.ColumnName]
		idx, ok := tableMeta.Indexs[index.IndexName]
		if ok {
			idx.Values = append(idx.Values, col)
		} else {
			index.Values = append(index.Values, col)
			tableMeta.Indexs[index.IndexName] = index
		}
	}
	if len(tableMeta.Indexs) == 0 {
		return types.TableMeta{}, errors.Errorf("Could not found any index in the table: %s", tableName)
	}

	return tableMeta, nil
}

// LoadAll
func (m *mysqlTrigger) LoadAll() ([]types.TableMeta, error) {
	return []types.TableMeta{}, nil
}

// getColumns
func (m *mysqlTrigger) getColumns(ctx context.Context, dbName string, table string, conn *sql.Conn) ([]types.ColumnMeta, error) {
	table = sqlUtil.DelEscape(table, types.DBTypeMySQL)

	result := make([]types.ColumnMeta, 0)

	stmt, err := conn.PrepareContext(ctx, constant.ColumnSchemaSql)
	if err != nil {
		return nil, err
	}

	rows, err := stmt.QueryContext(ctx, dbName, table)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var (
			tableCatalog string
			tableName    string
			tableSchema  string
			columnName   string
			dataType     string
			columnType   string
			columnKey    string
			isNullable   string
			extra        string
		)

		col := types.ColumnMeta{}

		if err = rows.Scan(
			&tableCatalog,
			&tableName,
			&tableSchema,
			&columnName,
			&dataType,
			&columnType,
			&columnKey,
			&isNullable,
			&extra); err != nil {
			return nil, err
		}

		col.Schema = tableSchema
		col.Table = tableName
		col.ColumnName = strings.Trim(columnName, "` ")
		col.DataType = sqlUtil.GetSqlDataType(dataType)
		col.ColumnType = columnType
		col.ColumnKey = columnKey
		if strings.ToLower(isNullable) == "yes" {
			col.IsNullable = 1
		} else {
			col.IsNullable = 0
		}
		col.Extra = extra
		col.Autoincrement = strings.Contains("auto_increment", strings.ToLower(extra))

		result = append(result, col)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	if err = rows.Close(); err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, errors.New("can't find column")
	}

	return result, nil
}

// getIndex
func (m *mysqlTrigger) getIndexes(ctx context.Context, dbName string, tableName string, conn *sql.Conn) ([]types.IndexMeta, error) {
	tableName = sqlUtil.DelEscape(tableName, types.DBTypeMySQL)

	result := make([]types.IndexMeta, 0)

	stmt, err := conn.PrepareContext(ctx, constant.IndexSchemaSql)
	if err != nil {
		return nil, err
	}

	rows, err := stmt.QueryContext(ctx, dbName, tableName)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var (
			indexName, columnName, nonUnique, indexType, collation string
			cardinality                                            int
		)

		if err = rows.Scan(
			&indexName,
			&columnName,
			&nonUnique,
			&indexType,
			&collation,
			&cardinality); err != nil {
			return nil, err
		}

		index := types.IndexMeta{
			Schema:     dbName,
			Table:      tableName,
			IndexName:  indexName,
			ColumnName: columnName,
			Values:     make([]types.ColumnMeta, 0),
		}

		if nonUnique == "1" || "yes" == strings.ToLower(nonUnique) {
			index.NonUnique = true
		}

		if "primary" == strings.ToLower(indexName) {
			index.IndexType = types.IndexPrimary
		} else if !index.NonUnique {
			index.IndexType = types.IndexUnique
		} else {
			index.IndexType = types.IndexNormal
		}

		result = append(result, index)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
