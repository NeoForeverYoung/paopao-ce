package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	SmallScale = 1000 // 小规模：1000用户
	DSN        = "paopao:paopao@tcp(127.0.0.1:3306)/paopao?charset=utf8mb4&parseTime=True&loc=Local"
)

type ContentType int

const (
	ContentTitle      ContentType = 1
	ContentText       ContentType = 2
	ContentImage      ContentType = 3
	ContentVideo      ContentType = 4
	ContentAudio      ContentType = 5
	ContentLink       ContentType = 6
	ContentFile       ContentType = 7
	ContentChargeFile ContentType = 8
)

var (
	usernames = []string{
		"热心网友", "吃瓜群众", "路人甲", "小明", "小红", "小李", "老王", "阿强", "晓雯", "大佬",
		"萌新", "咸鱼", "程序员", "设计师", "产品经理", "运营小姐姐", "技术宅", "二次元", "游戏玩家", "摄影师",
		"美食家", "旅行者", "读书人", "音乐人", "影评人", "段子手", "话痨", "沉默者", "观察家", "思考者",
	}

	postTitles = []string{
		"今天天气真好", "分享一个技术心得", "生活小确幸", "工作感悟", "学习笔记",
		"美食推荐", "旅行见闻", "电影评论", "书籍推荐", "音乐分享",
		"摄影作品", "设计灵感", "编程经验", "创业感悟", "投资心得",
		"健身打卡", "美妆心得", "育儿经验", "宠物日常", "游戏攻略",
	}

	postContents = []string{
		"今天阳光明媚，心情特别好！推荐大家也出去走走～",
		"刚学会了一个新的编程技巧，分享给大家。希望对初学者有帮助。",
		"生活中的小美好总是让人感动，珍惜当下的每一刻。",
		"工作虽然辛苦，但收获满满。成长的路上从不孤单。",
		"学而时习之，不亦说乎。今天又有新的收获。",
		"发现了一家超好吃的餐厅，味道正宗，价格实惠，强烈推荐！",
		"这次旅行让我见识到了不同的风土人情，收获良多。",
		"刚看完一部很棒的电影，剧情精彩，演技在线，值得一看。",
		"最近读了一本好书，分享一些读后感。",
		"这首歌太好听了，单曲循环中～",
		"今天拍了些照片，光线和构图都很满意。",
		"设计需要灵感，生活处处皆设计。",
		"代码就是艺术，简洁优雅的代码让人赏心悦目。",
		"创业路上充满挑战，但也充满机遇。",
		"理财有道，投资需谨慎。分享一些个人经验。",
	}

	tags = []string{
		"生活", "技术", "美食", "旅行", "电影", "读书", "音乐", "摄影", "设计", "编程",
		"创业", "投资", "健身", "美妆", "育儿", "宠物", "游戏", "学习", "工作", "感悟",
	}
)

func main() {
	fmt.Println("开始生成小规模测试数据（1000用户）...")

	// 连接数据库
	db, err := sql.Open("mysql", DSN)
	if err != nil {
		log.Fatal("连接数据库失败:", err)
	}
	defer db.Close()

	// 测试连接
	if err = db.Ping(); err != nil {
		log.Fatal("数据库连接测试失败:", err)
	}

	fmt.Println("数据库连接成功")

	// 随机种子
	rand.Seed(time.Now().UnixNano())

	// 获取当前最大ID
	var maxUserID, maxPostID int64
	db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM p_user").Scan(&maxUserID)
	db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM p_post").Scan(&maxPostID)

	fmt.Printf("当前最大用户ID: %d, 最大推文ID: %d\n", maxUserID, maxPostID)

	// 生成用户
	fmt.Println("正在生成用户数据...")
	userIDs := generateUsers(db, SmallScale, maxUserID)
	fmt.Printf("已生成 %d 个用户\n", len(userIDs))

	// 生成推文（每个用户平均3-5条推文）
	fmt.Println("正在生成推文数据...")
	postCount := generatePosts(db, userIDs, maxPostID)
	fmt.Printf("已生成 %d 条推文\n", postCount)

	// 生成关注关系（每个用户关注20-50个其他用户）
	fmt.Println("正在生成关注关系...")
	followCount := generateFollowings(db, userIDs)
	fmt.Printf("已生成 %d 个关注关系\n", followCount)

	// 生成好友关系（每个用户5-15个好友）
	fmt.Println("正在生成好友关系...")
	friendCount := generateFriends(db, userIDs)
	fmt.Printf("已生成 %d 个好友关系\n", friendCount)

	fmt.Println("数据生成完成！")

	// 统计最终数据
	showFinalStats(db)
}

func generateUsers(db *sql.DB, count int, startID int64) []int64 {
	userIDs := make([]int64, 0, count)

	stmt, err := db.Prepare(`
		INSERT INTO p_user (username, nickname, password, salt, status, avatar, balance, is_admin, is_del, created_on, modified_on)
		VALUES (?, ?, ?, ?, 1, '', 0, 0, 0, ?, ?)
	`)
	if err != nil {
		log.Fatal("准备用户插入语句失败:", err)
	}
	defer stmt.Close()

	now := time.Now().Unix()

	for i := 0; i < count; i++ {
		username := fmt.Sprintf("testuser_%d", startID+int64(i)+1)
		nickname := usernames[rand.Intn(len(usernames))] + fmt.Sprintf("_%d", i+1)
		password := "e10adc3949ba59abbe56e057f20f883e" // 123456 的 MD5
		salt := "paopao"

		result, err := stmt.Exec(username, nickname, password, salt, now, now)
		if err != nil {
			log.Printf("插入用户失败: %v", err)
			continue
		}

		id, _ := result.LastInsertId()
		userIDs = append(userIDs, id)

		if (i+1)%100 == 0 {
			fmt.Printf("已生成 %d 个用户\n", i+1)
		}
	}

	return userIDs
}

func generatePosts(db *sql.DB, userIDs []int64, startID int64) int {
	postStmt, err := db.Prepare(`
		INSERT INTO p_post (user_id, comment_count, collection_count, upvote_count, share_count, 
		                   visibility, is_top, is_essence, is_lock, latest_replied_on, tags, 
		                   attachment_price, ip, ip_loc, is_del, created_on, modified_on)
		VALUES (?, 0, 0, 0, 0, 90, 0, 0, 0, ?, ?, 0, '127.0.0.1', '本地', 0, ?, ?)
	`)
	if err != nil {
		log.Fatal("准备推文插入语句失败:", err)
	}
	defer postStmt.Close()

	contentStmt, err := db.Prepare(`
		INSERT INTO p_post_content (post_id, user_id, content, type, sort, is_del, created_on, modified_on)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)
	`)
	if err != nil {
		log.Fatal("准备推文内容插入语句失败:", err)
	}
	defer contentStmt.Close()

	postCount := 0
	now := time.Now().Unix()

	for _, userID := range userIDs {
		// 每个用户生成3-5条推文
		numPosts := rand.Intn(3) + 3

		for j := 0; j < numPosts; j++ {
			// 随机选择标签
			selectedTags := make([]string, 0, 3)
			tagIndices := rand.Perm(len(tags))[:rand.Intn(3)+1]
			for _, idx := range tagIndices {
				selectedTags = append(selectedTags, tags[idx])
			}
			tagsStr := ""
			if len(selectedTags) > 0 {
				for i, tag := range selectedTags {
					if i > 0 {
						tagsStr += ","
					}
					tagsStr += tag
				}
			}

			// 插入推文
			result, err := postStmt.Exec(userID, now, tagsStr, now, now)
			if err != nil {
				log.Printf("插入推文失败: %v", err)
				continue
			}

			postID, _ := result.LastInsertId()

			// 插入推文标题
			title := postTitles[rand.Intn(len(postTitles))]
			_, err = contentStmt.Exec(postID, userID, title, ContentTitle, 1, now, now)
			if err != nil {
				log.Printf("插入推文标题失败: %v", err)
			}

			// 插入推文内容
			content := postContents[rand.Intn(len(postContents))]
			_, err = contentStmt.Exec(postID, userID, content, ContentText, 2, now, now)
			if err != nil {
				log.Printf("插入推文内容失败: %v", err)
			}

			postCount++
		}

		if postCount%100 == 0 {
			fmt.Printf("已生成 %d 条推文\n", postCount)
		}
	}

	return postCount
}

func generateFollowings(db *sql.DB, userIDs []int64) int {
	stmt, err := db.Prepare(`
		INSERT IGNORE INTO p_following (user_id, follow_id, is_del, created_on, modified_on)
		VALUES (?, ?, 0, ?, ?)
	`)
	if err != nil {
		log.Fatal("准备关注关系插入语句失败:", err)
	}
	defer stmt.Close()

	followCount := 0
	now := time.Now().Unix()

	for _, userID := range userIDs {
		// 每个用户关注20-50个其他用户
		numFollows := rand.Intn(31) + 20

		// 随机选择要关注的用户
		otherUsers := make([]int64, 0, len(userIDs)-1)
		for _, id := range userIDs {
			if id != userID {
				otherUsers = append(otherUsers, id)
			}
		}

		// 如果其他用户不够，就关注所有其他用户
		if len(otherUsers) < numFollows {
			numFollows = len(otherUsers)
		}

		// 随机打乱并选择前numFollows个
		rand.Shuffle(len(otherUsers), func(i, j int) {
			otherUsers[i], otherUsers[j] = otherUsers[j], otherUsers[i]
		})

		for j := 0; j < numFollows; j++ {
			_, err := stmt.Exec(userID, otherUsers[j], now, now)
			if err == nil {
				followCount++
			}
		}

		if followCount%1000 == 0 {
			fmt.Printf("已生成 %d 个关注关系\n", followCount)
		}
	}

	return followCount
}

func generateFriends(db *sql.DB, userIDs []int64) int {
	stmt, err := db.Prepare(`
		INSERT IGNORE INTO p_contact (user_id, friend_id, group_id, remark, status, is_del, created_on, modified_on)
		VALUES (?, ?, 1, '', 2, 0, ?, ?)
	`)
	if err != nil {
		log.Fatal("准备好友关系插入语句失败:", err)
	}
	defer stmt.Close()

	friendCount := 0
	now := time.Now().Unix()

	for _, userID := range userIDs {
		// 每个用户5-15个好友
		numFriends := rand.Intn(11) + 5

		// 随机选择要加为好友的用户
		otherUsers := make([]int64, 0, len(userIDs)-1)
		for _, id := range userIDs {
			if id != userID {
				otherUsers = append(otherUsers, id)
			}
		}

		if len(otherUsers) < numFriends {
			numFriends = len(otherUsers)
		}

		rand.Shuffle(len(otherUsers), func(i, j int) {
			otherUsers[i], otherUsers[j] = otherUsers[j], otherUsers[i]
		})

		for j := 0; j < numFriends; j++ {
			friendID := otherUsers[j]

			// 双向好友关系
			_, err1 := stmt.Exec(userID, friendID, now, now)
			_, err2 := stmt.Exec(friendID, userID, now, now)

			if err1 == nil {
				friendCount++
			}
			if err2 == nil {
				friendCount++
			}
		}

		if friendCount%1000 == 0 {
			fmt.Printf("已生成 %d 个好友关系\n", friendCount)
		}
	}

	return friendCount
}

func showFinalStats(db *sql.DB) {
	fmt.Println("\n=== 最终数据统计 ===")

	var count int64

	db.QueryRow("SELECT COUNT(*) FROM p_user").Scan(&count)
	fmt.Printf("总用户数: %d\n", count)

	db.QueryRow("SELECT COUNT(*) FROM p_post").Scan(&count)
	fmt.Printf("总推文数: %d\n", count)

	db.QueryRow("SELECT COUNT(*) FROM p_post_content").Scan(&count)
	fmt.Printf("推文内容数: %d\n", count)

	db.QueryRow("SELECT COUNT(*) FROM p_following").Scan(&count)
	fmt.Printf("关注关系数: %d\n", count)

	db.QueryRow("SELECT COUNT(*) FROM p_contact WHERE status=2").Scan(&count)
	fmt.Printf("好友关系数: %d\n", count)
}
