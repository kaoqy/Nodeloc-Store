package http

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FlexibleTime 解析浏览器 <input type="datetime-local"> 与标准 RFC3339 两种写法。
//
// 前端的 datetime-local 只给到分钟（2026-10-03T09:00），没有秒也没有时区；
// 而 encoding/json 默认按 RFC3339 解析 time.Time，缺秒就直接报错——活动因此
// 永远保存不了。这里两种格式都接受，并且把没有时区的值当作本地时间处理。
type FlexibleTime struct {
	Time  time.Time
	Valid bool
}

// 前端可能送出的几种写法，按常见程度排列。
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

func (t *FlexibleTime) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		return nil
	}
	if strings.HasPrefix(raw, "\"") {
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil
		}
	}
	for _, layout := range timeLayouts {
		if layout == time.RFC3339 || layout == time.RFC3339Nano {
			if parsed, err := time.Parse(layout, raw); err == nil {
				t.Time = parsed
				t.Valid = true
				return nil
			}
			continue
		}
		// 不带时区的写法按服务器本地时区解释，与店主在表单里看到的一致。
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			t.Time = parsed
			t.Valid = true
			return nil
		}
	}
	return fmt.Errorf("无法识别的时间格式：%s", raw)
}

// MarshalJSON 输出标准 RFC3339，前端与其它接口拿到的格式保持一致。
func (t FlexibleTime) MarshalJSON() ([]byte, error) {
	if !t.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time)
}

// Ptr 返回可直接放进模型字段的指针。
func (t FlexibleTime) Ptr() *time.Time {
	if !t.Valid {
		return nil
	}
	value := t.Time
	return &value
}
