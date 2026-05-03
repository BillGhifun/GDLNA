package dlnadb

import (
	"GDLNA/dlnalogger"
	"GDLNA/system"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
	//_ "github.com/logoove/sqlite"
	"os"
	"reflect"
	"strings"
	"sync"
)

var MainDatabase *sqlx.DB

var MuMainWrite sync.RWMutex

//var MainDLNATable = make(map[string]*DLNATable)

var ListDlna MainMediaList

type MainMediaList struct {
	MediaList []MediaInfo `json:"dlna_list"`
}

type MediaInfo struct {
	Title     string `json:"o_name"`
	RuneTitle string `json:"r_name"`
	Link      string `json:"link"`
	Album     string `json:"album"`
	Artist    string `json:"artist"`
	Creator   string `json:"creator"`
	FileName  string `json:"filename"`
	Time      string `json:"time"`
}

//type DLNATable struct {
//	Link string
//	Name string
//	Info albumInfo
//	Time string
//}

type AlbumInfo struct {
	Album   string `json:"album"`
	Artist  string `json:"artist"`
	Creator string `json:"creator"`
}

func LoadMainDB() { // 建立一个默认的数据库
	// 删除文件 os.Remove("./gog.db")
	v := reflect.ValueOf(MainDatabase) // 数据库指针未初始化实体
	if !v.IsValid() || (v.Kind() == reflect.Ptr && v.IsNil()) {
		// 历史记录数据库未初始化
		//return
	} else {
		err := MainDatabase.Close()
		if err != nil {
			dlnalogger.Error(fmt.Sprintf("重载程序主数据库时发生错误: %s\n", err.Error()))
			return
		}
	}

	var bErr error
	////dsn := system.MainRootPath + "/db/dlna_link.db?cache=shared&mode=rwc"
	//dsn := "." + system.PathCharacter + "db" + system.PathCharacter + "dlna_link.db?cache=shared&mode=rwc"

	dbFile := filepath.Join(system.MainRootPath, "db", "dlna_link.db") // 先安全地生成纯文件路径
	dsn := fmt.Sprintf("file:%s?cache=shared&mode=rwc", dbFile)        // 拼接 SQLite 的 URI 协议前缀和参数 注意：SQLite 要求使用 "file:" 前缀来启用高级参数

	//dsn := system.MainRootPath + "/db/dlna_link.db"
	dlnalogger.Info(fmt.Sprintf("数据库路径: %s", dbFile))
	MainDatabase, bErr = sqlx.Open("sqlite", dsn) // 打开数据库
	if bErr != nil {
		dlnalogger.Error(fmt.Sprintf("打开程序主数据库时发生错误: %s\n", bErr.Error()))
		os.Exit(2000)
	}

	//============================================================================
	// 启用 WAL 模式
	_, err := MainDatabase.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		dlnalogger.Warning(fmt.Sprintf("无法设置数据库WAL: %s\n", err.Error()))
	}
	// 设置连接池
	// SetMaxIdleConns 设置空闲连接池中连接的最大数量
	//MainHisDatabase.SetMaxIdleConns(10)
	// SetMaxOpenConns 设置打开数据库连接的最大数量。
	//MainHisDatabase.SetMaxOpenConns(100)
	MainDatabase.SetMaxOpenConns(10)
	//MainHisDatabase.SetConnMaxLifetime(5 * time.Minute) // 连接的最大生命周期
	//============================================================================

	createDLNATable() // 建表

	loadCacheFromDB()
}

// getRemoteLink
func DLNALinkAdd(nLink MediaInfo) { // 增加一个
	ListDlna.MediaList = append(ListDlna.MediaList, nLink)
}

//func DLNALinkDel(nLink MediaInfo) { // 删除一条
//	delete(MainDLNATable, nLink.Link)
//}

func DLNALinkCls() { // 清空
	ListDlna = MainMediaList{} //make(map[string]*DLNATable)
}

var isFirstBuilt = true

func createDLNATable() {
	MuMainWrite.Lock()
	defer MuMainWrite.Unlock()
	sqlStmt := `create table gDLNA (Link TEXT PRIMARY KEY,
										Name TEXT,
										Info TEXT,
										TIME TEXT);`
	_, err := MainDatabase.Exec(sqlStmt)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			dlnalogger.Error(fmt.Sprintf("建立投屏数据库时发生错误: %s\n", err.Error()))
			os.Exit(2005)
		} else {
			isFirstBuilt = false
		}
	}
}

func loadCacheFromDB() {
	var tmpCount int

	// 清空现有缓存
	DLNALinkCls()

	// 只查询需要的字段
	rows, err := MainDatabase.Query("SELECT Link, Name, Info, TIME FROM gDLNA")
	if err != nil {
		dlnalogger.Error("打开数据库缓存表时发生错误")
		return
	}
	defer rows.Close()

	// 读取数据库数据
	MuMainWrite.Lock()
	defer MuMainWrite.Unlock()
	for rows.Next() {
		var dlnaRow MediaInfo
		var dlnaInfo string
		err = rows.Scan(&dlnaRow.Link, &dlnaRow.Title, &dlnaInfo, &dlnaRow.Time)
		if err != nil {
			dlnalogger.Error("循环读取数据库缓存表时发生错误")
			return
		}
		var tmpAlbumInfo AlbumInfo
		err = json.Unmarshal([]byte(dlnaInfo), &tmpAlbumInfo)
		if err != nil {
			return
		}
		dlnaRow.RuneTitle = system.GetRuneName(dlnaRow.Title)
		dlnaRow.Album = tmpAlbumInfo.Album
		dlnaRow.Artist = tmpAlbumInfo.Artist
		dlnaRow.Creator = tmpAlbumInfo.Creator
		dlnaRow.FileName = system.RemoveInvalidChars(dlnaRow.Title, dlnaRow.Time)
		//fmt.Println("到WEB列表:", dlnaRow.FileName, dlnaRow.Time)
		DLNALinkAdd(dlnaRow) // 增加一个
		tmpCount++
	}

	if rows.Err() != nil { // 检查是否有错误
		dlnalogger.Error(fmt.Sprintf("读取数据库缓存表数据时发生错误: %s", rows.Err().Error()))
		return
	}

	if !isFirstBuilt { // 首次建立数据库无需压缩
		_, err = MainDatabase.Exec("VACUUM;") // 执行 VACUUM 命令
		if err != nil {
			dlnalogger.Error(fmt.Sprintf("执行VACUUM优化时发生错误: %s\n", err.Error()))
			return
		}
	}

	dlnalogger.Info(fmt.Sprintf("装载数据:[%d]条", tmpCount))
}

func InsertDLnaData(sourceDLNA MediaInfo) {
	var tmpInfo AlbumInfo
	tmpInfo.Album = sourceDLNA.Album
	tmpInfo.Artist = sourceDLNA.Artist
	tmpInfo.Creator = sourceDLNA.Creator
	jsonIpListData, err := json.Marshal(tmpInfo)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("插入DLNA链接错误A: %s", err.Error()))
	}

	// SQL 插入语句，使用 INSERT OR REPLACE
	query := `INSERT INTO gDLNA (Link,Name,Info,TIME) VALUES (?,?,?,?)`

	MuMainWrite.Lock()
	defer MuMainWrite.Unlock()
	// 执行 SQL 插入语句
	_, err = MainDatabase.Exec(
		query,
		sourceDLNA.Link,
		sourceDLNA.Title,
		//sourceQuery.QueryIpList
		jsonIpListData,
		sourceDLNA.Time)
	//return err
	if err != nil {
		//dlnalogger.Error(err.Error())
		dlnalogger.Error(fmt.Sprintf("插入DLNA链接错误B: %s", err.Error()))
	}

}

func DeleteDLnaData(sLink string) bool {
	stmt, err := MainDatabase.Prepare("delete from gDLNA where Link=?")
	if err != nil {
		return false
	}
	defer stmt.Close()
	res, err := stmt.Exec(sLink)
	if err != nil {
		return false
	}
	_, err = res.RowsAffected()
	if err != nil {
		return false
	}
	return true
}

func ClearAllDLnaData() bool {
	stmt, err := MainDatabase.Prepare("delete from gDLNA")
	if err != nil {
		return false
	}
	defer stmt.Close()
	_, err = stmt.Exec()
	if err != nil {
		return false
	}
	return true
}

// AddMediaIfNotExists 检查链接是否已存在，如果不存在则原子地添加到数据库和缓存
// 返回 true 表示添加了新记录，false 表示已存在或出错
func AddMediaIfNotExists(media MediaInfo) bool {
	MuMainWrite.Lock()
	defer MuMainWrite.Unlock()

	// 检查链接是否已存在
	for _, m := range ListDlna.MediaList {
		if m.Link == media.Link {
			return false
		}
	}

	// 构造 AlbumInfo JSON
	var tmpInfo AlbumInfo
	tmpInfo.Album = media.Album
	tmpInfo.Artist = media.Artist
	tmpInfo.Creator = media.Creator
	jsonIpListData, err := json.Marshal(tmpInfo)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("AddMediaIfNotExists 序列化AlbumInfo错误: %s", err.Error()))
		return false
	}

	// 插入数据库
	query := `INSERT INTO gDLNA (Link,Name,Info,TIME) VALUES (?,?,?,?)`
	_, err = MainDatabase.Exec(query, media.Link, media.Title, jsonIpListData, media.Time)
	if err != nil {
		dlnalogger.Error(fmt.Sprintf("AddMediaIfNotExists 插入数据库错误: %s", err.Error()))
		return false
	}

	// 添加到缓存
	ListDlna.MediaList = append(ListDlna.MediaList, media)
	return true
}
