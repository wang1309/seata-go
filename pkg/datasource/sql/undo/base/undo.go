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

package base

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/seata/seata-go/pkg/datasource/sql/datasource/mysql"
	"github.com/seata/seata-go/pkg/datasource/sql/undo/factor"
	"github.com/seata/seata-go/pkg/datasource/sql/undo/impl"

	"github.com/pkg/errors"
	"github.com/seata/seata-go/pkg/common/log"
	"github.com/seata/seata-go/pkg/common/util"
	"github.com/seata/seata-go/pkg/constant"
	"github.com/seata/seata-go/pkg/datasource/sql/undo"

	"github.com/seata/seata-go/pkg/datasource/sql/types"
)

var _ undo.UndoLogManager = (*BaseUndoLogManager)(nil)

var ErrorDeleteUndoLogParamsFault = errors.New("xid or branch_id can't nil")

const (
	PairSplit = "&"
	KvSplit   = "="

	DeleteUndoLogSql = constant.DeleteFrom + constant.UndoLogTableName + " WHERE " + constant.UndoLogBranchXid + " = ? AND " + constant.UndoLogXid + " = ?"
	SelectUndoLogSql = "SELECT `log_status`,`context`,`rollback_info` FROM " + constant.UndoLogTableName + " WHERE " + constant.UndoLogBranchXid + " = ? AND " + constant.UndoLogXid + " = ? FOR UPDATE"
)

// BaseUndoLogManager
type BaseUndoLogManager struct{}

func NewBaseUndoLogManager() *BaseUndoLogManager {
	return &BaseUndoLogManager{}
}

// Init
func (m *BaseUndoLogManager) Init() {
}

// InsertUndoLog
func (m *BaseUndoLogManager) InsertUndoLog(l []impl.BranchUndoLog, tx driver.Tx) error {
	return nil
}

// DeleteUndoLog exec delete single undo log operate
func (m *BaseUndoLogManager) DeleteUndoLog(ctx context.Context, xid string, branchID int64, conn *sql.Conn) error {
	stmt, err := conn.PrepareContext(ctx, DeleteUndoLogSql)
	if err != nil {
		log.Errorf("[DeleteUndoLog] prepare sql fail, err: %v", err)
		return err
	}

	if _, err = stmt.ExecContext(ctx, branchID, xid); err != nil {
		log.Errorf("[DeleteUndoLog] exec delete undo log fail, err: %v", err)
		return err
	}

	return nil
}

// BatchDeleteUndoLog exec delete undo log operate
func (m *BaseUndoLogManager) BatchDeleteUndoLog(xid []string, branchID []int64, conn *sql.Conn) error {
	// build delete undo log sql
	batchDeleteSql, err := m.getBatchDeleteUndoLogSql(xid, branchID)
	if err != nil {
		log.Errorf("get undo sql log fail, err: %v", err)
		return err
	}

	ctx := context.Background()

	// prepare deal sql
	stmt, err := conn.PrepareContext(ctx, batchDeleteSql)
	if err != nil {
		log.Errorf("prepare sql fail, err: %v", err)
		return err
	}

	branchIDStr, err := util.Int64Slice2Str(branchID, ",")
	if err != nil {
		log.Errorf("slice to string transfer fail, err: %v", err)
		return err
	}

	// exec sql stmt
	if _, err = stmt.ExecContext(ctx, branchIDStr, strings.Join(xid, ",")); err != nil {
		log.Errorf("exec delete undo log fail, err: %v", err)
		return err
	}

	return nil
}

// FlushUndoLog
func (m *BaseUndoLogManager) FlushUndoLog(txCtx *types.TransactionContext, tx driver.Tx) error {
	return nil
}

// Undo
func (m *BaseUndoLogManager) Undo(
	ctx context.Context,
	xid string,
	branchID int64,
	conn *sql.Conn) error {

	return nil
}

// RunUndo
func (m *BaseUndoLogManager) RunUndo(
	ctx context.Context,
	dbType types.DBType,
	xid string,
	branchID int64,
	conn *sql.Conn) error {

	branchUndoLogs := make([]impl.BranchUndoLog, 0)

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			if err = tx.Rollback(); err != nil {
				log.Errorf("[RunUndo] rollback fail, xid: %s, branchID:%s err:%v", xid, branchID, err)
				return
			}
		}
	}()

	stmt, err := tx.Prepare(SelectUndoLogSql)
	if err != nil {
		log.Errorf("[Undo] prepare sql fail, err: %v", err)
		return err
	}

	defer func() {
		if err = stmt.Close(); err != nil {
			log.Errorf("[RunUndo] stmt close fail, xid: %s, branchID:%s err:%v", xid, branchID, err)
			return
		}
	}()

	rows, err := stmt.Query(branchID, xid)
	if err != nil {
		log.Errorf("[Undo] query sql fail, err: %v", err)
		return err
	}

	exist := false
	for rows.Next() {
		exist = true
		var (
			logStatus    string
			context      string
			rollbackInfo []byte
		)

		if err = rows.Scan(&logStatus, &context, &rollbackInfo); err != nil {
			log.Errorf("[Undo] get log status fail, err: %v", err)
			return err
		}

		state, _ := strconv.Atoi(logStatus)

		// check if it can undo
		if !m.canUndo(state) {
			return nil
		}

		// Todo pr 242 调用对应的 parser 方法
		/*contextMap := m.parseContext(context)
		rollbackInfo := m.getRollbackInfo(rollbackInfo, contextMap)
		serializer := m.getSerializer(contextMap)
		branchUndoLog = parser.decode(rollbackInfo);
		*/

		// Todo 替换成 parser 解析器解析
		var branchUndoLog impl.BranchUndoLog
		if err = json.Unmarshal(rollbackInfo, &branchUndoLog); err != nil {
			return err
		}

		branchUndoLogs = append(branchUndoLogs, branchUndoLog)
	}

	if err = rows.Err(); err != nil {
		return err
	}

	if err = rows.Close(); err != nil {
		return err
	}

	for _, branchUndoLog := range branchUndoLogs {
		sqlUndoLogs := branchUndoLog.Logs
		if len(sqlUndoLogs) > 1 {
			m.Reverse(sqlUndoLogs)
		}

		for _, undoLog := range sqlUndoLogs {
			tableMeta, err := mysql.GetTableMetaInstance().GetTableMeta(ctx, undoLog.TableName, conn)
			if err != nil {
				log.Errorf("[Undo] get table meta fail, err: %v", err)
				return err
			}

			undoLog.SetTableMeta(tableMeta)

			undoExecutor, err := factor.GetUndoExecutor(dbType, undoLog)
			if err != nil {
				log.Errorf("[Undo] get undo executor, err: %v", err)
				return err
			}

			if err = undoExecutor.ExecuteOn(ctx, dbType, undoLog, conn); err != nil {
				log.Errorf("[Undo] execute on fail, err: %v", err)
				return err
			}
		}
	}

	if exist {
		if err = m.DeleteUndoLog(ctx, xid, branchID, conn); err != nil {
			log.Errorf("[Undo] delete undo log fail, err: %v", err)
			return err
		}
	}
	// Todo 等 insertLog 合并后加上 insertUndoLogWithGlobalFinished 功能
	/*else {

	}*/

	if err = tx.Commit(); err != nil {
		log.Errorf("[Undo] execute on fail, err: %v", err)
		return nil
	}

	return nil
}

// DBType
func (m *BaseUndoLogManager) DBType() types.DBType {
	panic("implement me")
}

// getBatchDeleteUndoLogSql build batch delete undo log
func (m *BaseUndoLogManager) getBatchDeleteUndoLogSql(xid []string, branchID []int64) (string, error) {
	if len(xid) == 0 || len(branchID) == 0 {
		return "", ErrorDeleteUndoLogParamsFault
	}

	var undoLogDeleteSql strings.Builder
	undoLogDeleteSql.WriteString(constant.DeleteFrom)
	undoLogDeleteSql.WriteString(constant.UndoLogTableName)
	undoLogDeleteSql.WriteString(" WHERE ")
	undoLogDeleteSql.WriteString(constant.UndoLogBranchXid)
	undoLogDeleteSql.WriteString(" IN ")
	m.appendInParam(len(branchID), &undoLogDeleteSql)
	undoLogDeleteSql.WriteString(" AND ")
	undoLogDeleteSql.WriteString(constant.UndoLogXid)
	undoLogDeleteSql.WriteString(" IN ")
	m.appendInParam(len(xid), &undoLogDeleteSql)

	return undoLogDeleteSql.String(), nil
}

// appendInParam build in param
func (m *BaseUndoLogManager) appendInParam(size int, str *strings.Builder) {
	if size <= 0 {
		return
	}

	str.WriteString(" (")
	for i := 0; i < size; i++ {
		str.WriteString("?")
		if i < size-1 {
			str.WriteString(",")
		}
	}

	str.WriteString(") ")
}

// canUndo check if it can undo
func (m *BaseUndoLogManager) canUndo(state int) bool {
	return state == constant.UndoLogStatusNormal
}

// parseContext
func (m *BaseUndoLogManager) parseContext(str string) map[string]string {
	return m.DecodeMap(str)
}

// DecodeMap Decode undo log context string to map
func (m *BaseUndoLogManager) DecodeMap(str string) map[string]string {
	res := make(map[string]string)

	if str == "" {
		return nil
	}

	strSlice := strings.Split(str, PairSplit)
	if len(strSlice) == 0 {
		return nil
	}

	for key, _ := range strSlice {
		kv := strings.Split(strSlice[key], KvSplit)
		if len(kv) != 2 {
			continue
		}

		res[kv[0]] = kv[1]
	}

	return res
}

// getRollbackInfo
func (m *BaseUndoLogManager) getRollbackInfo(rollbackInfo []byte, undoContext map[string]string) []byte {
	// Todo 目前 insert undo log 未实现压缩功能，实现后补齐这块功能
	// get compress type
	/*compressorType, ok := undoContext[constant.CompressorTypeKey]
	if ok {

	}*/

	return rollbackInfo
}

// getSerializer get serializer from undo context
func (m *BaseUndoLogManager) getSerializer(undoLogContext map[string]string) (serializer string) {
	if undoLogContext == nil {
		return
	}

	serializer, _ = undoLogContext[constant.SerializerKey]
	return
}

// Reverse Reverses the order of the elements in the specified slice
func (m *BaseUndoLogManager) Reverse(log []impl.SQLUndoLog) {
	if len(log) == 0 {
		return
	}

	left, right := 0, len(log)-1

	for left < right {
		log[left], log[right] = log[right], log[left]
		left++
		right--
	}
}
