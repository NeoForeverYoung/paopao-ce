// Copyright 2022 ROC. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package web

import (
	"fmt"

	"github.com/gin-gonic/gin"
	api "github.com/rocboss/paopao-ce/auto/api/v1"
	"github.com/rocboss/paopao-ce/internal/conf"
	"github.com/rocboss/paopao-ce/internal/core"
	"github.com/rocboss/paopao-ce/internal/core/cs"
	"github.com/rocboss/paopao-ce/internal/core/ms"
	"github.com/rocboss/paopao-ce/internal/dao/jinzhu/dbr"
	"github.com/rocboss/paopao-ce/internal/model/joint"
	"github.com/rocboss/paopao-ce/internal/model/web"
	"github.com/rocboss/paopao-ce/internal/servants/base"
	"github.com/rocboss/paopao-ce/internal/servants/chain"
	"github.com/sirupsen/logrus"
)

var (
	// इंश्योर looseSrv реализует интерфейс api.Loose
	// Это проверка времени компиляции, чтобы убедиться, что все методы интерфейса реализованы.
	_ api.Loose = (*looseSrv)(nil)
)

// looseSrv 实现了 "宽松" 权限的服务，主要处理公开的、不需要严格授权的API请求
type looseSrv struct {
	// 嵌入未实现的 LooseServant 以确保向前兼容
	api.UnimplementedLooseServant
	// 嵌入基础的DAO服务，包含数据库连接等
	*base.DaoServant
	// ac 是应用级别的缓存实例，用于缓存API响应
	ac core.AppCache
	// userTweetsExpire 是用户动态列表的缓存过期时间
	userTweetsExpire int64
	// idxTweetsExpire 是首页动态列表的缓存过期时间
	idxTweetsExpire int64
	// tweetCommentsExpire 是动态评论的缓存过期时间
	tweetCommentsExpire int64
	// prefixUserTweets 是用户动态缓存键的前缀
	prefixUserTweets string
	// prefixIdxTweetsNewest 是首页最新动态缓存键的前缀
	prefixIdxTweetsNewest string
	// prefixIdxTweetsHots 是首页热门动态缓存键的前缀
	prefixIdxTweetsHots string
	// prefixIdxTweetsFollowing 是首页关注动态缓存键的前缀
	prefixIdxTweetsFollowing string
	// prefixTweetComment 是动态评论缓存键的前缀
	prefixTweetComment string
}

// Chain 返回应用到此服务所有路由的中间件链
func (s *looseSrv) Chain() gin.HandlersChain {
	// JwtLoose 是一个宽松的JWT中间件，它会尝试解析JWT
	// 如果成功，用户信息会存入context；如果失败，则继续执行，适用于公开接口
	return gin.HandlersChain{chain.JwtLoose()}
}

// Timeline 处理获取动态时间线的请求，包括首页和搜索结果
func (s *looseSrv) Timeline(req *web.TimelineReq) (*web.TimelineResp, error) {
	// 计算分页参数
	limit, offset := req.PageSize, (req.Page-1)*req.PageSize

	// 根据请求参数判断是获取首页动态还是执行搜索
	// 如果没有查询关键词(Query)但类型(Type)是"search"，则视为获取首页动态
	if req.Query == "" && req.Type == "search" {
		return s.getIndexTweets(req, limit, offset)
	}

	// 如果有查询关键词，则执行搜索逻辑
	q := &core.QueryReq{
		Query: req.Query,
		Type:  core.SearchType(req.Type),
	}
	// 调用搜索服务执行搜索
	res, err := s.Ts.Search(req.User, q, offset, limit)
	if err != nil {
		logrus.Errorf("Ts.Search err: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 对搜索结果进行二次处理，例如补充用户信息等
	posts, err := s.Ds.RevampPosts(res.Items)
	if err != nil {
		logrus.Errorf("Ds.RevampPosts err: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 获取当前登录用户的ID，如果未登录则为-1
	userId := int64(-1)
	if req.User != nil {
		userId = req.User.ID
	}
	// 准备推文的附加信息，如当前用户是否点赞、收藏等
	if err := s.PrepareTweets(userId, posts); err != nil {
		logrus.Errorf("timeline occurs error[2]: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 构建分页响应
	resp := joint.PageRespFrom(posts, req.Page, req.PageSize, res.Total)
	// 封装最终的API响应
	return &web.TimelineResp{
		CachePageResp: joint.CachePageResp{
			Data: resp,
		},
	}, nil
}

// getIndexTweets 处理获取首页动态列表的逻辑（非搜索）
func (s *looseSrv) getIndexTweets(req *web.TimelineReq, limit int, offset int) (res *web.TimelineResp, err error) {
	// 尝试从缓存中获取数据，如果成功则直接返回
	key, ok := "", false
	if res, key, ok = s.indexTweetsFromCache(req, limit, offset); ok {
		// logrus.Debugf("getIndexTweets from cache key:%s", key)
		return
	}

	// 缓存未命中，从数据库查询
	var (
		posts []*ms.Post
		total int64
		xerr  error
	)
	// 根据请求的样式（style）查询不同类型的动态
	switch req.Style {
	case web.StyleTweetsFollowing: // 获取关注的人的动态
		if req.User != nil {
			posts, total, xerr = s.Ds.ListFollowingTweets(req.User.ID, limit, offset)
		} else {
			// 未登录用户请求关注动态，降级为获取最新动态
			// 这种情况可能发生在前端用户退出登录后立即刷新页面
			posts, total, xerr = s.Ds.ListIndexNewestTweets(limit, offset)
		}
	case web.StyleTweetsNewest: // 获取全站最新动态
		posts, total, xerr = s.Ds.ListIndexNewestTweets(limit, offset)
	case web.StyleTweetsHots: // 获取全站热门动态
		posts, total, xerr = s.Ds.ListIndexHotsTweets(limit, offset)
	default: // 未知的样式
		return nil, web.ErrGetPostsUnknowStyle
	}

	// 检查数据库查询错误
	if xerr != nil {
		logrus.Errorf("getIndexTweets occurs error[1]: %s", xerr)
		return nil, web.ErrGetPostFailed
	}

	// 将数据库原始的Post模型列表，合并转换成包含完整信息的PostFormated模型列表
	postsFormated, verr := s.Ds.MergePosts(posts)
	if verr != nil {
		logrus.Errorf("getIndexTweets in merge posts occurs error: %s", verr)
		return nil, web.ErrGetPostFailed
	}

	// 获取当前登录用户ID
	userId := int64(-1)
	if req.User != nil {
		userId = req.User.ID
	}
	// 准备推文的附加信息（点赞、收藏状态等）
	if err := s.PrepareTweets(userId, postsFormated); err != nil {
		logrus.Errorf("getIndexTweets occurs error[2]: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 构建分页响应
	resp := joint.PageRespFrom(postsFormated, req.Page, req.PageSize, total)
	// 将从数据库获取的结果存入缓存
	base.OnCacheRespEvent(s.ac, key, resp, s.idxTweetsExpire)
	// 封装最终的API响应
	return &web.TimelineResp{
		CachePageResp: joint.CachePageResp{
			Data: resp,
		},
	}, nil
}

// indexTweetsFromCache 尝试从缓存中获取首页动态
func (s *looseSrv) indexTweetsFromCache(req *web.TimelineReq, limit int, offset int) (res *web.TimelineResp, key string, ok bool) {
	// 如果是游客，用户名为"_"，否则为登录用户名
	username := "_"
	if req.User != nil {
		username = req.User.Username
	}

	// 根据动态样式和分页参数构建唯一的缓存键(key)
	// 注意：关注页的缓存键包含了用户名，因为每个用户关注的人不同
	switch req.Style {
	case web.StyleTweetsFollowing:
		key = fmt.Sprintf("%s%s:%d:%d", s.prefixIdxTweetsFollowing, username, offset, limit)
	case web.StyleTweetsNewest:
		key = fmt.Sprintf("%s%s:%d:%d", s.prefixIdxTweetsNewest, username, offset, limit)
	case web.StyleTweetsHots:
		key = fmt.Sprintf("%s%s:%d:%d", s.prefixIdxTweetsHots, username, offset, limit)
	default:
		// 未知样式，直接返回，不使用缓存
		return
	}

	// 尝试从缓存中获取数据
	if data, err := s.ac.Get(key); err == nil {
		// 缓存命中
		ok, res = true, &web.TimelineResp{
			CachePageResp: joint.CachePageResp{
				JsonResp: data,
			},
		}
	}
	// 返回结果，key和ok状态
	return
}

// tweetCommentsFromCache 尝试从缓存中获取动态的评论列表
func (s *looseSrv) tweetCommentsFromCache(req *web.TweetCommentsReq, limit int, offset int) (res *web.TweetCommentsResp, key string, ok bool) {
	// 根据动态ID、评论样式和分页信息构建唯一的缓存键
	key = fmt.Sprintf("%s%d:%s:%d:%d", s.prefixTweetComment, req.TweetId, req.Style, limit, offset)

	// 尝试获取缓存
	if data, err := s.ac.Get(key); err == nil {
		// 缓存命中
		ok, res = true, &web.TweetCommentsResp{
			CachePageResp: joint.CachePageResp{
				JsonResp: data,
			},
		}
	}
	return
}

// GetUserTweets 获取指定用户的动态列表
func (s *looseSrv) GetUserTweets(req *web.GetUserTweetsReq) (res *web.GetUserTweetsResp, err error) {
	// 获取访问者相对于目标用户的关系类型（例如：自己、好友、关注、游客等）
	user, xerr := s.RelationTypFrom(req.User, req.Username)
	if xerr != nil {
		return nil, err
	}

	// 尝试从缓存中获取数据
	key, ok := "", false
	if res, key, ok = s.userTweetsFromCache(req, user); ok {
		// logrus.Debugf("GetUserTweets from cache key:%s", key)
		return
	}

	// 缓存未命中，从数据库查询
	switch req.Style {
	case web.UserPostsStyleComment: // 获取用户评论过的动态
		fallthrough
	case web.UserPostsStyleMedia: // 获取用户发布的多媒体动态
		res, err = s.listUserTweets(req, user)
	case web.UserPostsStyleHighlight: // 获取用户的精华动态
		res, err = s.getUserPostTweets(req, user, true)
	case web.UserPostsStyleStar: // 获取用户点赞的动态
		res, err = s.getUserStarTweets(req, user)
	case web.UserPostsStylePost: // 获取用户发布的动态（默认）
		fallthrough
	default:
		res, err = s.getUserPostTweets(req, user, false)
	}

	// 如果数据库查询成功，则将结果写入缓存
	if err == nil {
		base.OnCacheRespEvent(s.ac, key, res.Data, s.userTweetsExpire)
	}
	return
}

// userTweetsFromCache 尝试从缓存中获取用户动态列表
func (s *looseSrv) userTweetsFromCache(req *web.GetUserTweetsReq, user *cs.VistUser) (res *web.GetUserTweetsResp, key string, ok bool) {
	// 根据不同样式构建缓存键
	switch req.Style {
	case web.UserPostsStylePost, web.UserPostsStyleHighlight, web.UserPostsStyleMedia:
		// 这几种样式下，内容的可见性取决于访问者与作者的关系，所以缓存键包含关系类型(RelTyp)
		key = fmt.Sprintf("%s%d:%s:%s:%d:%d", s.prefixUserTweets, user.UserId, req.Style, user.RelTyp, req.Page, req.PageSize)
	default:
		// 其他样式下，内容的可见性取决于访问者本身（比如"我"评论过的），所以缓存键包含访问者用户名
		meName := "_"
		if user.RelTyp != cs.RelationGuest {
			meName = req.User.Username
		}
		key = fmt.Sprintf("%s%d:%s:%s:%d:%d", s.prefixUserTweets, user.UserId, req.Style, meName, req.Page, req.PageSize)
	}

	// 尝试获取缓存
	if data, err := s.ac.Get(key); err == nil {
		ok, res = true, &web.GetUserTweetsResp{
			CachePageResp: joint.CachePageResp{
				JsonResp: data,
			},
		}
	}
	return
}

// getUserStarTweets 获取用户点赞的动态列表
func (s *looseSrv) getUserStarTweets(req *web.GetUserTweetsReq, user *cs.VistUser) (*web.GetUserTweetsResp, error) {
	// 从数据库查询用户点赞的动态
	stars, totalRows, err := s.Ds.ListUserStarTweets(user, req.PageSize, (req.Page-1)*req.PageSize)
	if err != nil {
		logrus.Errorf("getUserStarTweets err[1]: %s", err)
		return nil, web.ErrGetStarsFailed
	}

	// 从点赞记录中提取出动态(Post)本身
	var posts []*ms.Post
	for _, star := range stars {
		if star.Post != nil {
			posts = append(posts, star.Post)
		}
	}

	// 合并动态的完整信息
	postsFormated, err := s.Ds.MergePosts(posts)
	if err != nil {
		logrus.Errorf("Ds.MergePosts err: %s", err)
		return nil, web.ErrGetStarsFailed
	}

	// 获取当前登录用户ID
	userId := int64(-1)
	if req.User != nil {
		userId = req.User.ID
	}
	// 准备推文的附加信息
	if err := s.PrepareTweets(userId, postsFormated); err != nil {
		logrus.Errorf("getUserStarTweets err[2]: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 构建分页响应
	resp := joint.PageRespFrom(postsFormated, req.Page, req.PageSize, totalRows)
	return &web.GetUserTweetsResp{
		CachePageResp: joint.CachePageResp{
			Data: resp,
		},
	}, nil
}

// listUserTweets 获取用户评论过的或发布的多媒体动态
func (s *looseSrv) listUserTweets(req *web.GetUserTweetsReq, user *cs.VistUser) (*web.GetUserTweetsResp, error) {
	var (
		tweets []*ms.Post
		total  int64
		err    error
	)
	// 根据样式分发到不同的数据库查询方法
	if req.Style == web.UserPostsStyleComment {
		tweets, total, err = s.Ds.ListUserCommentTweets(user, req.PageSize, (req.Page-1)*req.PageSize)
	} else if req.Style == web.UserPostsStyleMedia {
		tweets, total, err = s.Ds.ListUserMediaTweets(user, req.PageSize, (req.Page-1)*req.PageSize)
	} else {
		logrus.Errorf("s.listUserTweets unknow style[1]: %s", req.Style)
		return nil, web.ErrGetPostsFailed
	}

	if err != nil {
		logrus.Errorf("s.listUserTweets err[2]: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 合并动态的完整信息
	postsFormated, err := s.Ds.MergePosts(tweets)
	if err != nil {
		logrus.Errorf("s.listUserTweets err[3]: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 获取当前登录用户ID
	userId := int64(-1)
	if req.User != nil {
		userId = req.User.ID
	}
	// 准备推文的附加信息
	if err := s.PrepareTweets(userId, postsFormated); err != nil {
		logrus.Errorf("s.listUserTweets err[4]: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 构建分页响应
	resp := joint.PageRespFrom(postsFormated, req.Page, req.PageSize, total)
	return &web.GetUserTweetsResp{
		CachePageResp: joint.CachePageResp{
			Data: resp,
		},
	}, nil
}

// getUserPostTweets 获取用户发布的动态（包括精华）
func (s *looseSrv) getUserPostTweets(req *web.GetUserTweetsReq, user *cs.VistUser, isHighlight bool) (*web.GetUserTweetsResp, error) {
	// 根据访问者与作者的关系，确定查询时使用的可见性风格
	style := cs.StyleUserTweetsGuest
	switch user.RelTyp {
	case cs.RelationAdmin:
		style = cs.StyleUserTweetsAdmin
	case cs.RelationSelf:
		style = cs.StyleUserTweetsSelf
	case cs.RelationFriend:
		style = cs.StyleUserTweetsFriend
	case cs.RelationFollowing:
		style = cs.StyleUserTweetsFollowing
	case cs.RelationGuest:
		fallthrough
	default:
		// 默认为游客风格
	}

	// 调用DAO层获取用户动态列表
	posts, total, err := s.Ds.ListUserTweets(user.UserId, style, isHighlight, req.PageSize, (req.Page-1)*req.PageSize)
	if err != nil {
		logrus.Errorf("s.GetTweetList error[1]: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 合并动态的完整信息
	postsFormated, xerr := s.Ds.MergePosts(posts)
	if xerr != nil {
		logrus.Errorf("s.GetTweetList error[2]: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 获取当前登录用户ID
	userId := int64(-1)
	if req.User != nil {
		userId = req.User.ID
	}
	// 准备推文的附加信息
	if err := s.PrepareTweets(userId, postsFormated); err != nil {
		logrus.Errorf("s.GetTweetList error[3]: %s", err)
		return nil, web.ErrGetPostsFailed
	}

	// 构建分页响应
	resp := joint.PageRespFrom(postsFormated, req.Page, req.PageSize, total)
	return &web.GetUserTweetsResp{
		CachePageResp: joint.CachePageResp{
			Data: resp,
		},
	}, nil
}

// GetUserProfile 获取用户公开的个人主页信息
func (s *looseSrv) GetUserProfile(req *web.GetUserProfileReq) (*web.GetUserProfileResp, error) {
	// 通过用户名获取用户基本信息
	he, err := s.Ds.UserProfileByName(req.Username)
	if err != nil {
		logrus.Errorf("looseSrv.GetUserProfile occurs error[1]: %s", err)
		return nil, web.ErrNoExistUsername
	}

	// 判断是否为好友，默认不是自己的朋友
	isFriend := !(req.User == nil || req.User.ID == he.ID)
	// 如果是其他用户，则查询好友关系
	if req.User != nil && req.User.ID != he.ID {
		isFriend = s.Ds.IsFriend(req.User.ID, he.ID)
	}

	// 判断是否已关注
	isFollowing := false
	if req.User != nil {
		isFollowing = s.Ds.IsFollow(req.User.ID, he.ID)
	}

	// 获取用户的关注数和粉丝数
	follows, followings, err := s.Ds.GetFollowCount(he.ID)
	if err != nil {
		return nil, web.ErrGetPostsFailed
	}

	// 组装最终的响应数据
	return &web.GetUserProfileResp{
		ID:          he.ID,
		Nickname:    he.Nickname,
		Username:    he.Username,
		Status:      he.Status,
		Avatar:      he.Avatar,
		IsAdmin:     he.IsAdmin,
		IsFriend:    isFriend,
		IsFollowing: isFollowing,
		CreatedOn:   he.CreatedOn,
		Follows:     follows,
		Followings:  followings,
		TweetsCount: he.TweetsCount,
	}, nil
}

// TopicList 获取话题标签列表
func (s *looseSrv) TopicList(req *web.TopicListReq) (*web.TopicListResp, error) {
	var (
		tags, extralTags cs.TagList
		err              error
	)
	num := req.Num
	// 根据请求类型获取不同的话题列表
	switch req.Type {
	case web.TagTypeHot: // 热门话题
		tags, err = s.Ds.GetHotTags(req.Uid, num, 0)
	case web.TagTypeNew: // 最新话题
		tags, err = s.Ds.GetNewestTags(req.Uid, num, 0)
	case web.TagTypeFollow: // 我关注的话题
		tags, err = s.Ds.GetFollowTags(req.Uid, false, num, 0)
	case web.TagTypePin: // 我置顶的话题
		tags, err = s.Ds.GetFollowTags(req.Uid, true, num, 0)
	case web.TagTypeHotExtral: // 获取热门话题，并额外获取我关注的话题
		extralNum := req.ExtralNum
		if extralNum <= 0 {
			extralNum = num
		}
		tags, err = s.Ds.GetHotTags(req.Uid, num, 0)
		if err == nil {
			extralTags, err = s.Ds.GetFollowTags(req.Uid, false, extralNum, 0)
		}
	default:
		err = web.ErrGetPostTagsFailed
	}

	if err != nil {
		return nil, web.ErrGetPostTagsFailed
	}

	// 组装响应
	return &web.TopicListResp{
		Topics:       tags,
		ExtralTopics: extralTags,
	}, nil
}

// TweetComments 获取动态的评论列表
func (s *looseSrv) TweetComments(req *web.TweetCommentsReq) (res *web.TweetCommentsResp, err error) {
	limit, offset := req.PageSize, (req.Page-1)*req.PageSize

	// 尝试从缓存获取
	key, ok := "", false
	if res, key, ok = s.tweetCommentsFromCache(req, limit, offset); ok {
		logrus.Debugf("looseSrv.TweetComments from cache key:%s", key)
		return
	}

	// 缓存未命中，从数据库查询主评论
	comments, totalRows, xerr := s.Ds.GetComments(req.TweetId, req.Style.ToInnerValue(), limit, offset)
	if xerr != nil {
		logrus.Errorf("looseSrv.TweetComments occurs error[1]: %s", xerr)
		return nil, web.ErrGetCommentsFailed
	}

	// 批量获取评论相关的ID，用于后续的批量查询
	userIDs := []int64{}
	commentIDs := []int64{}
	for _, comment := range comments {
		userIDs = append(userIDs, comment.UserID)
		commentIDs = append(commentIDs, comment.ID)
	}

	// 批量获取评论作者的用户信息
	users, xerr := s.Ds.GetUsersByIDs(userIDs)
	if xerr != nil {
		logrus.Errorf("looseSrv.TweetComments occurs error[2]: %s", xerr)
		return nil, web.ErrGetCommentsFailed
	}

	// 批量获取评论的内容（针对图文混排的评论）
	contents, xerr := s.Ds.GetCommentContentsByIDs(commentIDs)
	if xerr != nil {
		logrus.Errorf("looseSrv.TweetComments occurs error[3]: %s", xerr)
		return nil, web.ErrGetCommentsFailed
	}

	// 批量获取评论的回复
	replies, xerr := s.Ds.GetCommentRepliesByID(commentIDs)
	if xerr != nil {
		logrus.Errorf("looseSrv.TweetComments occurs error[4]: %s", xerr)
		return nil, web.ErrGetCommentsFailed
	}

	// 如果用户已登录，批量获取用户对评论和回复的点赞状态
	var commentThumbs, replyThumbs cs.CommentThumbsMap
	if req.Uid > 0 {
		commentThumbs, replyThumbs, xerr = s.Ds.GetCommentThumbsMap(req.Uid, req.TweetId)
		if xerr != nil {
			logrus.Errorf("looseSrv.TweetComments occurs error[5]: %s", xerr)
			return nil, web.ErrGetCommentsFailed
		}
	}

	// 将回复按评论ID分组，并附加上点赞信息
	replyMap := make(map[int64][]*dbr.CommentReplyFormated)
	if len(replyThumbs) > 0 {
		for _, reply := range replies {
			if thumbs, exist := replyThumbs[reply.ID]; exist {
				reply.IsThumbsUp, reply.IsThumbsDown = thumbs.IsThumbsUp, thumbs.IsThumbsDown
			}
			replyMap[reply.CommentID] = append(replyMap[reply.CommentID], reply)
		}
	} else {
		for _, reply := range replies {
			replyMap[reply.CommentID] = append(replyMap[reply.CommentID], reply)
		}
	}

	// 组装最终的评论列表，将用户信息、内容、回复、点赞信息都合并到每个评论对象中
	commentsFormated := []*ms.CommentFormated{}
	for _, comment := range comments {
		commentFormated := comment.Format()
		// 合并评论的点赞信息
		if thumbs, exist := commentThumbs[comment.ID]; exist {
			commentFormated.IsThumbsUp, commentFormated.IsThumbsDown = thumbs.IsThumbsUp, thumbs.IsThumbsDown
		}
		// 合并评论的图文内容
		for _, content := range contents {
			if content.CommentID == comment.ID {
				commentFormated.Contents = append(commentFormated.Contents, content)
			}
		}
		// 合并评论的回复列表
		if replySlice, exist := replyMap[commentFormated.ID]; exist {
			commentFormated.Replies = replySlice
		}
		// 合并评论的作者信息
		for _, user := range users {
			if user.ID == comment.UserID {
				commentFormated.User = user.Format()
			}
		}
		commentsFormated = append(commentsFormated, commentFormated)
	}
	// 构建分页响应
	resp := joint.PageRespFrom(commentsFormated, req.Page, req.PageSize, totalRows)
	// 将结果写入缓存
	base.OnCacheRespEvent(s.ac, key, resp, s.tweetCommentsExpire)
	return &web.TweetCommentsResp{
		CachePageResp: joint.CachePageResp{
			Data: resp,
		},
	}, nil
}

// TweetDetail 获取单条动态的详情
func (s *looseSrv) TweetDetail(req *web.TweetDetailReq) (*web.TweetDetailResp, error) {
	// 获取动态主体
	post, err := s.Ds.GetPostByID(req.TweetId)
	if err != nil {
		return nil, web.ErrGetPostFailed
	}
	// 获取动态的图文内容
	postContents, err := s.Ds.GetPostContentsByIDs([]int64{post.ID})
	if err != nil {
		return nil, web.ErrGetPostFailed
	}
	// 获取动态作者的用户信息
	users, err := s.Ds.GetUsersByIDs([]int64{post.UserID})
	if err != nil {
		return nil, web.ErrGetPostFailed
	}

	// 数据整合，将用户信息和内容整合到动态主体中
	postFormated := post.Format()
	for _, user := range users {
		postFormated.User = user.Format()
	}
	for _, content := range postContents {
		if content.PostID == post.ID {
			postFormated.Contents = append(postFormated.Contents, content.Format())
		}
	}

	// 准备动态的附加信息（点赞、收藏状态等）
	if err = s.PrepareTweet(req.User, postFormated); err != nil {
		return nil, web.ErrGetPostFailed
	}

	// 核心逻辑：检测当前用户是否有权限查看此动态
	// TODO: 这个逻辑应该提到最前面，避免无效的数据库查询
	switch {
	case req.User != nil && (req.User.ID == postFormated.User.ID || req.User.IsAdmin):
		// 作者本人或管理员，可以查看
		break
	case post.Visibility == core.PostVisitPublic:
		// 公开动态，可以查看
		break
	case post.Visibility == core.PostVisitFriend && postFormated.User.IsFriend:
		// 好友可见动态，且当前用户是好友，可以查看
		break
	case post.Visibility == core.PostVisitFollowing && postFormated.User.IsFollowing:
		// 关注者可见动态，且当前用户已关注，可以查看
		break
	default:
		// 其他情况，无权限
		return nil, web.ErrNoPermission
	}
	return (*web.TweetDetailResp)(postFormated), nil
}

// newLooseSrv 创建一个新的 looseSrv 实例
func newLooseSrv(s *base.DaoServant, ac core.AppCache) api.Loose {
	cs := conf.CacheSetting
	return &looseSrv{
		DaoServant:               s,
		ac:                       ac,
		userTweetsExpire:         cs.UserTweetsExpire,
		idxTweetsExpire:          cs.IndexTweetsExpire,
		tweetCommentsExpire:      cs.TweetCommentsExpire,
		prefixUserTweets:         conf.PrefixUserTweets,
		prefixIdxTweetsNewest:    conf.PrefixIdxTweetsNewest,
		prefixIdxTweetsHots:      conf.PrefixIdxTweetsHots,
		prefixIdxTweetsFollowing: conf.PrefixIdxTweetsFollowing,
		prefixTweetComment:       conf.PrefixTweetComment,
	}
}
