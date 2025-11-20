package subtitle

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func parseTimeToSeconds(time string) (float64, error) {
	parts := strings.Split(time, ":")
	if len(parts) == 3 {

		hours, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, err
		}
		minutes, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, err
		}
		seconds, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, err
		}

		return float64(hours*3600+minutes*60) + seconds, nil
	} else if len(parts) == 2 {
		minutes, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, err
		}
		seconds, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}

		return float64(minutes*60) + seconds, nil
	}
	return 0, fmt.Errorf("invalid time format: %s", time)
}

func parseVTTTotalDuration(vttFile string) (float64, error) {
	file, err := os.Open(vttFile)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	//re := regexp.MustCompile(`(\d{2}:\d{2}:\d{2}\.\d{3}) --> (\d{2}:\d{2}:\d{2}\.\d{3})`)
	var endTime float64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matches := strings.Split(line, " --> ")
		if len(matches) == 2 {
			parsedEndTime, err := parseTimeToSeconds(matches[1])
			if err != nil {
				continue
			}
			// Yangi eng katta tugash vaqtini saqlash
			if parsedEndTime > endTime {
				endTime = parsedEndTime
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return endTime, nil
}

func CreateM3U8FromVTT(path, vttFile string) error {
	duration, err := parseVTTTotalDuration(path + vttFile) // Yangi funksiya chaqiriladi
	if err != nil {
		return err
	}

	m3u8Content := fmt.Sprintf(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:YES
#EXT-X-TARGETDURATION:%d
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:%.1f,
%s
#EXT-X-ENDLIST
`, int(duration), duration, vttFile)

	file, err := os.Create(path + "index.m3u8")
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(m3u8Content)
	if err != nil {
		return err
	}

	return nil
}
