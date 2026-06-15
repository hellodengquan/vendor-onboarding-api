package utils

import (
	"fmt"
	"time"
)

func GenerateVendorCode() string {
	now := time.Now()
	return fmt.Sprintf("V%s%06d", now.Format("20060102150405"), time.Now().UnixNano()%1000000)
}
