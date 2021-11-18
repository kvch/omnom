package webapp

import (
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/asciimoo/omnom/config"
	"github.com/asciimoo/omnom/model"

	"github.com/gin-gonic/gin"

	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/contrib/sessions"
)

const (
	SERVER_ADDR string = ":7331"
	SID         string = "sid"
)

var e *gin.Engine

var tplFuncMap = template.FuncMap{
	"HasPrefix": strings.HasPrefix,
	"ToHTML":    func(s string) template.HTML { return template.HTML(s) },
	"ToAttr":    func(s string) template.HTMLAttr { return template.HTMLAttr(s) },
	"ToURL":     func(s string) template.URL { return template.URL(s) },
	"ToDate":    func(t time.Time) string { return t.Format("2006-01-02") },
}

func createRenderer() multitemplate.Renderer {
	r := multitemplate.DynamicRender{}
	r.AddFromFilesFuncs("index", tplFuncMap, "templates/layout/base.tpl", "templates/index.tpl")
	r.AddFromFilesFuncs("signup", tplFuncMap, "templates/layout/base.tpl", "templates/signup.tpl")
	r.AddFromFilesFuncs("signup-confirm", tplFuncMap, "templates/layout/base.tpl", "templates/signup_confirm.tpl")
	r.AddFromFilesFuncs("login", tplFuncMap, "templates/layout/base.tpl", "templates/login.tpl")
	r.AddFromFilesFuncs("login-confirm", tplFuncMap, "templates/layout/base.tpl", "templates/login_confirm.tpl")
	r.AddFromFilesFuncs("bookmarks", tplFuncMap, "templates/layout/base.tpl", "templates/bookmarks.tpl")
	r.AddFromFilesFuncs("my-bookmarks", tplFuncMap, "templates/layout/base.tpl", "templates/my_bookmarks.tpl")
	r.AddFromFilesFuncs("profile", tplFuncMap, "templates/layout/base.tpl", "templates/profile.tpl")
	return r
}

func renderHTML(c *gin.Context, status int, page string, vars map[string]interface{}) {
	session := sessions.Default(c)
	u, _ := c.Get("user")
	tplVars := gin.H{
		"Page": page,
		"User": u,
	}
	sessChanged := false
	if s := session.Get("Error"); s != nil {
		tplVars["Error"] = s.(string)
		session.Delete("Error")
		sessChanged = true
	}
	if s := session.Get("Warning"); s != nil {
		tplVars["Warning"] = s.(string)
		session.Delete("Warning")
		sessChanged = true
	}
	if s := session.Get("Info"); s != nil {
		tplVars["Info"] = s.(string)
		session.Delete("Info")
		sessChanged = true
	}
	if sessChanged {
		session.Save()
	}
	for k, v := range vars {
		tplVars[k] = v
	}
	c.HTML(status, page, tplVars)
}

func Run(cfg *config.Config) {
	e = gin.Default()
	if !cfg.App.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	e.HTMLRender = createRenderer()
	e.Use(sessions.Sessions("SID", sessions.NewCookieStore([]byte("secret"))))
	e.Use(SessionMiddleware())
	authorized := e.Group("/")
	authorized.Use(authRequired)

	// ROUTES
	e.Static("/static", "./static")
	e.GET("/", index)
	e.GET("/signup", signup)
	e.POST("/signup", signup)
	e.GET("/login", login)
	e.POST("/login", login)
	e.GET("/logout", logout)
	e.GET("/bookmarks", bookmarks)
	e.GET("/snapshot", snapshot)
	e.POST("/add_bookmark", addBookmark)

	authorized.GET("/profile", profile)
	authorized.GET("/generate_addon_token", generateAddonToken)
	authorized.GET("/delete_addon_token", deleteAddonToken)
	authorized.GET("/my_bookmarks", myBookmarks)

	log.Println("Starting server")
	e.Run(cfg.Server.Address)
}

func index(c *gin.Context) {
	renderHTML(c, http.StatusOK, "index", nil)
}

func signup(c *gin.Context) {
	if c.Request.Method == "POST" {
		username := c.PostForm("username")
		// TODO username format check
		email := c.PostForm("email")
		if username == "" || email == "" {
			renderHTML(c, http.StatusOK, "signup", map[string]interface{}{
				"Error": "Missing data",
			})
			return
		}
		u := model.GetUser(username)
		if u != nil {
			renderHTML(c, http.StatusOK, "signup", map[string]interface{}{
				"Error": "Username already exists",
			})
			return
		}
		err := model.CreateUser(username, email)
		if err != nil {
			renderHTML(c, http.StatusOK, "signup", map[string]interface{}{
				"Error": err,
			})
			return
		}
		log.Println("New extension token generated:", u.SubmissionTokens[0])

		renderHTML(c, http.StatusOK, "signup-confirm", nil)
		return
	}
	renderHTML(c, http.StatusOK, "signup", nil)
}

func login(c *gin.Context) {
	uname, ok := c.GetPostForm("username")
	if ok {
		u := model.GetUser(uname)
		if u == nil {
			renderHTML(c, http.StatusOK, "login", map[string]interface{}{
				"Error": "Unknown user",
			})
			return
		}
		u.LoginToken = model.GenerateToken()
		err := model.DB.Save(u).Error
		if err != nil {
			renderHTML(c, http.StatusOK, "login", map[string]interface{}{
				"Error": err,
			})
			return
		}
		log.Println("New login token generated:", u.LoginToken)
		renderHTML(c, http.StatusOK, "login-confirm", nil)
		return
	}

	tok, ok := c.GetQuery("token")
	if ok && tok != "" {
		u := model.GetUserByLoginToken(tok)
		if u == nil {
			renderHTML(c, http.StatusOK, "login", map[string]interface{}{
				"Error": "Invalid token",
			})
			return
		}
		u.LoginToken = ""
		err := model.DB.Save(u).Error
		if err != nil {
			renderHTML(c, http.StatusOK, "login", map[string]interface{}{
				"Error": err,
			})
			return
		}
		session := sessions.Default(c)
		session.Set(SID, u.Username)
		err = session.Save()
		if err != nil {
			renderHTML(c, http.StatusOK, "login", map[string]interface{}{
				"Error": err,
			})
			return
		}
		c.Redirect(http.StatusFound, "/")
		return
	}
	renderHTML(c, http.StatusOK, "login", nil)
}

func logout(c *gin.Context) {
	session := sessions.Default(c)
	user := session.Get(SID)
	if user == nil {
		c.Redirect(http.StatusFound, "/")
		return
	}
	session.Delete(SID)
	session.Save()
	c.Redirect(http.StatusFound, "/")
}

func profile(c *gin.Context) {
	u, _ := c.Get("user")
	tplData := map[string]interface{}{}
	if u == nil {
		c.Redirect(http.StatusFound, "/")
		return
	}
	var ts []*model.Token
	err := model.DB.Where("user_id = ?", u.(*model.User).ID).Find(&ts).Error
	if err != nil {
		tplData["Error"] = err.Error()
	}
	tplData["AddonTokens"] = ts
	renderHTML(c, http.StatusOK, "profile", tplData)
}

func snapshot(c *gin.Context) {
	id, ok := c.GetQuery("id")
	if !ok {
		return
	}
	var s *model.Snapshot
	err := model.DB.Where("id = ?", id).First(&s).Error
	if err != nil {
		return
	}
	var b *model.Bookmark
	err = model.DB.Where("id = ?", s.BookmarkID).First(&b).Error
	if err != nil {
		return
	}
	u, _ := c.Get("user")
	if !b.Public && (u == nil || u.(*model.User).ID != b.UserID) {
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(s.Site))
}

func bookmarks(c *gin.Context) {
	var bs []*model.Bookmark
	model.DB.Where("bookmarks.public = 1").Preload("Snapshots").Order("created_at desc").Find(&bs)
	renderHTML(c, http.StatusOK, "bookmarks", map[string]interface{}{
		"Bookmarks": bs,
	})
}

func myBookmarks(c *gin.Context) {
	u, _ := c.Get("user")
	var bs []*model.Bookmark
	model.DB.Model(u).Preload("Snapshots").Order("created_at desc").Association("Bookmarks").Find(&bs)
	renderHTML(c, http.StatusOK, "my-bookmarks", map[string]interface{}{
		"Bookmarks": bs,
	})
}

func addBookmark(c *gin.Context) {
	tok := c.PostForm("token")
	u := model.GetUserBySubmissionToken(tok)
	if u == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "invalid token",
		})
		return
	}
	b := &model.Bookmark{
		Title:   c.PostForm("title"),
		URL:     c.PostForm("url"),
		Notes:   c.PostForm("notes"),
		Favicon: c.PostForm("favicon"),
		UserID:  u.ID,
	}
	if c.PostForm("public") != "" {
		b.Public = true
	}
	snapshot := c.PostForm("snapshot")
	if snapshot != "" {
		b.Snapshots = []model.Snapshot{
			model.Snapshot{
				Site: snapshot,
			},
		}
	}
	model.DB.Save(b)
	c.Redirect(http.StatusFound, "/")
}

func generateAddonToken(c *gin.Context) {
	session := sessions.Default(c)
	u, _ := c.Get("user")
	tok := &model.Token{
		Text:   model.GenerateToken(),
		UserID: u.(*model.User).ID,
	}
	err := model.DB.Create(tok).Error
	if err != nil {
		session.Set("Error", err.Error())
	} else {
		session.Set("Info", "Token created")
	}
	session.Save()
	c.Redirect(http.StatusFound, "/profile")
}

func deleteAddonToken(c *gin.Context) {
	session := sessions.Default(c)
	id, _ := c.GetQuery("id")
	u, _ := c.Get("user")
	err := model.DB.Where("user_id = ? AND id = ?", u.(*model.User).ID, id).Delete(&model.Token{}).Error
	if err != nil {
		session.Set("Error", err.Error())
	} else {
		session.Set("Info", "Token deleted")
	}
	session.Save()
	c.Redirect(http.StatusFound, "/profile")
}

func authRequired(c *gin.Context) {
	session := sessions.Default(c)
	user := session.Get(SID)
	if user == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}
	c.Next()
}

func SessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		uname := session.Get(SID)
		if uname != nil {
			c.Set("user", model.GetUser(uname.(string)))
		} else {
			c.Set("user", nil)
		}
		c.Next()
	}
}
