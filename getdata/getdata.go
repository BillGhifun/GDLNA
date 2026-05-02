package getdata

import (
	"regexp"
	"strings"
)

// MetaData 定义结构体来映射XML元素
type MetaData struct {
	Title     string
	RuneTitle string
}

func GetMetaData(metaData string) MetaData {
	tmpMetaData := MetaData{}
	// 定义正则表达式
	re := regexp.MustCompile(`<dc:title>(.*?)</dc:title>`)
	// 查找匹配
	tmpTitles := re.FindStringSubmatch(metaData)
	if len(tmpTitles) > 1 {
		tmpName := strings.TrimSpace(tmpTitles[1])
		// 输出提取的内容
		tmpMetaData.Title = tmpName
		//fmt.Println("标题:", tmpTitle[1])
		// ... existing code ...
		// 将字符串转换为rune切片
		runes := []rune(tmpName)
		// 截取前10个字符并添加省略号
		if len(runes) > 10 {
			tmpMetaData.RuneTitle = string(runes[:10]) + "..."
		} else {
			tmpMetaData.RuneTitle = string(runes) // 如果字符少于10个，保持原样
		}
	} else {
		tmpMetaData.Title = "投屏视频"
		tmpMetaData.RuneTitle = "投屏视频"
	}
	return tmpMetaData
}
