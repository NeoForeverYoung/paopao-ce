package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

const (
	DSN = "paopao:paopao@tcp(127.0.0.1:3306)/paopao?charset=utf8mb4&parseTime=True&loc=Local"
)

type MySQLStatus struct {
	Connections                  int64
	MaxConnections               int64
	ThreadsConnected             int64
	ThreadsRunning               int64
	Questions                    int64
	Queries                      int64
	SlowQueries                  int64
	OpenTables                   int64
	QueriesPerSecond             float64
	ConnectionsPerSecond         float64
	UptimeSeconds                int64
	InnodbBufferPoolReads        int64
	InnodbBufferPoolReadRequests int64
	InnodbBufferPoolHitRate      float64
}

type TableStats struct {
	TableName   string
	TableRows   int64
	DataLength  int64
	IndexLength int64
	TotalSize   int64
}

func main() {
	fmt.Println("🔍 开始监控paopao-ce数据库性能...")

	// 连接数据库
	db, err := sql.Open("mysql", DSN)
	if err != nil {
		log.Fatal("连接数据库失败:", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal("数据库连接测试失败:", err)
	}

	fmt.Println("数据库连接成功\n")

	// 获取MySQL状态
	status, err := getMySQLStatus(db)
	if err != nil {
		log.Printf("获取MySQL状态失败: %v", err)
	} else {
		printMySQLStatus(status)
	}

	// 获取表统计信息
	fmt.Println("\n📊 主要表统计信息")
	fmt.Println("========================================")

	mainTables := []string{"p_user", "p_post", "p_post_content", "p_following", "p_contact"}
	for _, tableName := range mainTables {
		stats, err := getTableStats(db, tableName)
		if err != nil {
			log.Printf("获取表 %s 统计信息失败: %v", tableName, err)
			continue
		}
		printTableStats(stats)
	}

	// 检查索引使用情况
	fmt.Println("\n🔍 索引分析")
	fmt.Println("========================================")
	checkIndexes(db)

	// 检查慢查询
	fmt.Println("\n⏰ 慢查询分析")
	fmt.Println("========================================")
	checkSlowQueries(db)

	// 性能建议
	fmt.Println("\n💡 性能优化建议")
	fmt.Println("========================================")
	givePerformanceAdvice(status)
}

func getMySQLStatus(db *sql.DB) (*MySQLStatus, error) {
	status := &MySQLStatus{}

	// 获取基本状态变量
	statusVars := map[string]*int64{
		"Connections":                      &status.Connections,
		"Max_used_connections":             &status.MaxConnections,
		"Threads_connected":                &status.ThreadsConnected,
		"Threads_running":                  &status.ThreadsRunning,
		"Questions":                        &status.Questions,
		"Queries":                          &status.Queries,
		"Slow_queries":                     &status.SlowQueries,
		"Open_tables":                      &status.OpenTables,
		"Uptime":                           &status.UptimeSeconds,
		"Innodb_buffer_pool_reads":         &status.InnodbBufferPoolReads,
		"Innodb_buffer_pool_read_requests": &status.InnodbBufferPoolReadRequests,
	}

	for varName, ptr := range statusVars {
		var value sql.NullInt64
		err := db.QueryRow("SHOW STATUS LIKE ?", varName).Scan(new(string), &value)
		if err != nil {
			log.Printf("获取状态变量 %s 失败: %v", varName, err)
			continue
		}
		if value.Valid {
			*ptr = value.Int64
		}
	}

	// 计算派生指标
	if status.UptimeSeconds > 0 {
		status.QueriesPerSecond = float64(status.Queries) / float64(status.UptimeSeconds)
		status.ConnectionsPerSecond = float64(status.Connections) / float64(status.UptimeSeconds)
	}

	if status.InnodbBufferPoolReadRequests > 0 {
		status.InnodbBufferPoolHitRate = float64(status.InnodbBufferPoolReadRequests-status.InnodbBufferPoolReads) /
			float64(status.InnodbBufferPoolReadRequests) * 100
	}

	return status, nil
}

func getTableStats(db *sql.DB, tableName string) (*TableStats, error) {
	stats := &TableStats{TableName: tableName}

	query := `
		SELECT 
			TABLE_ROWS,
			DATA_LENGTH,
			INDEX_LENGTH,
			DATA_LENGTH + INDEX_LENGTH as TOTAL_SIZE
		FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = 'paopao' AND TABLE_NAME = ?
	`

	err := db.QueryRow(query, tableName).Scan(
		&stats.TableRows,
		&stats.DataLength,
		&stats.IndexLength,
		&stats.TotalSize,
	)

	return stats, err
}

func checkIndexes(db *sql.DB) {
	// 检查主要查询的索引情况
	queries := []struct {
		name  string
		query string
	}{
		{
			name:  "p_post表索引检查",
			query: "SHOW INDEX FROM p_post",
		},
		{
			name:  "p_following表索引检查",
			query: "SHOW INDEX FROM p_following",
		},
		{
			name:  "p_contact表索引检查",
			query: "SHOW INDEX FROM p_contact",
		},
	}

	for _, q := range queries {
		fmt.Printf("\n📋 %s:\n", q.name)
		rows, err := db.Query(q.query)
		if err != nil {
			log.Printf("执行查询失败: %v", err)
			continue
		}
		defer rows.Close()

		indexCount := 0
		for rows.Next() {
			var table, nonUnique, keyName, seqInIndex, columnName, collation, cardinality, subPart, packed, null, indexType, comment, indexComment string
			err := rows.Scan(&table, &nonUnique, &keyName, &seqInIndex, &columnName, &collation, &cardinality, &subPart, &packed, &null, &indexType, &comment, &indexComment)
			if err != nil {
				log.Printf("读取索引信息失败: %v", err)
				continue
			}
			if indexCount < 5 { // 只显示前5个索引
				fmt.Printf("   - 索引: %s, 列: %s, 类型: %s\n", keyName, columnName, indexType)
			}
			indexCount++
		}
		if indexCount > 5 {
			fmt.Printf("   ... 还有 %d 个索引\n", indexCount-5)
		}
	}
}

func checkSlowQueries(db *sql.DB) {
	// 检查慢查询设置
	var logSlowQueries, longQueryTime string

	err := db.QueryRow("SHOW VARIABLES LIKE 'slow_query_log'").Scan(new(string), &logSlowQueries)
	if err == nil {
		fmt.Printf("慢查询日志: %s\n", logSlowQueries)
	}

	err = db.QueryRow("SHOW VARIABLES LIKE 'long_query_time'").Scan(new(string), &longQueryTime)
	if err == nil {
		fmt.Printf("慢查询阈值: %s 秒\n", longQueryTime)
	}

	// 获取当前慢查询数量
	var slowQueries sql.NullInt64
	err = db.QueryRow("SHOW STATUS LIKE 'Slow_queries'").Scan(new(string), &slowQueries)
	if err == nil && slowQueries.Valid {
		fmt.Printf("累计慢查询数: %d\n", slowQueries.Int64)
	}
}

func printMySQLStatus(status *MySQLStatus) {
	fmt.Println("🗄️  MySQL状态信息")
	fmt.Println("========================================")
	fmt.Printf("连接数: %d / 最大连接数: %d\n", status.ThreadsConnected, status.MaxConnections)
	fmt.Printf("运行线程数: %d\n", status.ThreadsRunning)
	fmt.Printf("总连接数: %d (%.2f/秒)\n", status.Connections, status.ConnectionsPerSecond)
	fmt.Printf("总查询数: %d (%.2f QPS)\n", status.Queries, status.QueriesPerSecond)
	fmt.Printf("慢查询数: %d\n", status.SlowQueries)
	fmt.Printf("打开表数: %d\n", status.OpenTables)
	fmt.Printf("运行时间: %d 秒 (%.1f 小时)\n", status.UptimeSeconds, float64(status.UptimeSeconds)/3600)

	if status.InnodbBufferPoolReadRequests > 0 {
		fmt.Printf("InnoDB缓冲池命中率: %.2f%%\n", status.InnodbBufferPoolHitRate)
	}
}

func printTableStats(stats *TableStats) {
	fmt.Printf("\n📋 表: %s\n", stats.TableName)
	fmt.Printf("   行数: %d\n", stats.TableRows)
	fmt.Printf("   数据大小: %.2f MB\n", float64(stats.DataLength)/1024/1024)
	fmt.Printf("   索引大小: %.2f MB\n", float64(stats.IndexLength)/1024/1024)
	fmt.Printf("   总大小: %.2f MB\n", float64(stats.TotalSize)/1024/1024)
}

func givePerformanceAdvice(status *MySQLStatus) {
	fmt.Println("基于当前状态的优化建议:")

	// 连接数分析
	if status.ThreadsConnected > status.MaxConnections*80/100 {
		fmt.Println("⚠️  连接数接近上限，建议增加max_connections")
	} else {
		fmt.Println("✅ 连接数正常")
	}

	// 缓冲池命中率分析
	if status.InnodbBufferPoolHitRate < 95 {
		fmt.Printf("⚠️  InnoDB缓冲池命中率较低(%.2f%%)，建议增加innodb_buffer_pool_size\n", status.InnodbBufferPoolHitRate)
	} else {
		fmt.Printf("✅ InnoDB缓冲池命中率良好(%.2f%%)\n", status.InnodbBufferPoolHitRate)
	}

	// 慢查询分析
	if status.SlowQueries > 0 {
		fmt.Printf("⚠️  发现 %d 个慢查询，建议优化SQL或添加索引\n", status.SlowQueries)
	} else {
		fmt.Println("✅ 没有慢查询")
	}

	// QPS分析
	if status.QueriesPerSecond > 1000 {
		fmt.Printf("🔥 QPS很高(%.2f)，系统处理能力强\n", status.QueriesPerSecond)
	} else if status.QueriesPerSecond > 100 {
		fmt.Printf("👍 QPS良好(%.2f)\n", status.QueriesPerSecond)
	} else {
		fmt.Printf("💡 QPS较低(%.2f)，可能是轻负载或需要优化\n", status.QueriesPerSecond)
	}

	fmt.Println("\n💡 针对feeds流的优化建议:")
	fmt.Println("1. 为p_post表的user_id字段添加索引（如果还没有）")
	fmt.Println("2. 为p_post表的visibility + latest_replied_on添加复合索引")
	fmt.Println("3. 为p_following表的user_id字段添加索引")
	fmt.Println("4. 为p_contact表的friend_id + status添加复合索引")
	fmt.Println("5. 考虑使用Redis缓存热门推文列表")
	fmt.Println("6. 考虑分页查询的LIMIT偏移量优化")
}
