// Package service — ICS 订阅日历
package service

import (
	"fmt"
	"time"

	ics "github.com/arran4/golang-ical"

	"ani-rss-go/internal/entity"
	"ani-rss-go/internal/store"
)

// calendarZone 日历时区 Asia/Shanghai（无夏令时，使用固定偏移避免依赖系统时区数据）
var calendarZone = time.FixedZone("Asia/Shanghai", 8*60*60)

var icsDays = [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

// GenerateIcs 生成已启用订阅的 ICS 日历
func GenerateIcs() string {
	cal := ics.NewCalendarFor("ani-rss")
	cal.SetProductId("-//ani-rss//CN")
	cal.SetCalscale("GREGORIAN")
	cal.SetMethod(ics.MethodPublish)
	cal.SetXWRCalName("ani-rss")
	cal.SetXWRTimezone("Asia/Shanghai")
	cal.SetXWRCalDesc("ani-rss 动漫订阅日历")

	now := time.Now()
	for _, ani := range store.ANIList() {
		if !ani.GetEnable() {
			continue
		}
		addIcsEvent(cal, ani, now)
	}
	return cal.Serialize()
}

// addIcsEvent 生成订阅的全天事件，非剧场版按周重复至预计完结日期
func addIcsEvent(cal *ics.Calendar, ani *entity.Ani, now time.Time) {
	ova := ani.GetOva()
	typeLabel := "TV"
	if ova {
		typeLabel = "剧场版/OVA"
	}
	release := ani.ReleaseDate.In(calendarZone)
	start := time.Date(release.Year(), release.Month(), release.Day(), 0, 0, 0, 0, calendarZone)

	event := cal.AddEvent("ani-rss-" + ani.ID + "@ani-rss")
	event.SetAllDayStartAt(start)
	event.SetAllDayEndAt(start.AddDate(0, 0, 1))
	event.SetSummary(fmt.Sprintf("%s 第%d季", ani.Title, ani.Season))
	event.SetURL(ani.BgmUrl)
	event.SetDtStampTime(now)
	event.SetModifiedAt(now)
	event.SetStatus(ics.ObjectStatusConfirmed)
	event.SetTimeTransparency(ics.TransparencyTransparent)
	for _, category := range []string{"动漫", fmt.Sprintf("第%d季", ani.Season), typeLabel} {
		event.AddProperty(ics.ComponentPropertyCategories, category)
	}
	event.SetDescription(fmt.Sprintf("标题: %s\n类型: %s\n季度: %d\nBangumi: %s\n", ani.Title, typeLabel, ani.Season, ani.BgmUrl))

	if ova {
		return
	}
	// 一周一集计算结束时间，没有总集数时默认持续一个月
	until := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 1, 0)
	if ani.TotalEpisodeNumber > 0 {
		until = ani.ReleaseDate.AddDate(0, 0, 7*(ani.TotalEpisodeNumber-1))
	}
	event.AddRrule(fmt.Sprintf("FREQ=WEEKLY;BYDAY=%s;UNTIL=%s", icsDays[ani.ReleaseDate.Weekday()], until.Format("20060102")))
}
