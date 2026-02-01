package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"iac-platform/internal/models"
)

// FreezeScheduleService handles freeze schedule checking logic
type FreezeScheduleService struct{}

// NewFreezeScheduleService creates a new freeze schedule service
func NewFreezeScheduleService() *FreezeScheduleService {
	return &FreezeScheduleService{}
}

// IsInFreezeWindow checks if the current time is within any freeze schedule
// Returns (inFreeze, reason)
func (s *FreezeScheduleService) IsInFreezeWindow(schedules []models.FreezeSchedule) (bool, string) {
	if len(schedules) == 0 {
		return false, ""
	}

	now := time.Now()
	currentWeekday := int(now.Weekday())
	if currentWeekday == 0 {
		currentWeekday = 7 // Convert Sunday from 0 to 7
	}

	currentTime := now.Format("15:04")

	for _, schedule := range schedules {
		// Check if current weekday is in the schedule
		if !containsInt(schedule.Weekdays, currentWeekday) {
			continue
		}

		// Check if current time is within the freeze window
		if s.isTimeInWindow(currentTime, schedule.FromTime, schedule.ToTime) {
			return true, fmt.Sprintf("Freeze window: %s - %s on %s",
				schedule.FromTime, schedule.ToTime, s.formatWeekdays(schedule.Weekdays))
		}
	}

	return false, ""
}

// IsInFreezeWindowWithUnfreeze checks if the current time is within any freeze schedule
// but also checks for one-time unfreeze bypass
// Returns (inFreeze, reason)
func (s *FreezeScheduleService) IsInFreezeWindowWithUnfreeze(schedules []models.FreezeSchedule, unfreezeUntil *time.Time) (bool, string) {
	// Check if one-time unfreeze is active
	if unfreezeUntil != nil && time.Now().Before(*unfreezeUntil) {
		return false, "One-time unfreeze active"
	}

	// Otherwise check normal freeze schedules
	return s.IsInFreezeWindow(schedules)
}

// isTimeInWindow checks if the given time is within the from-to window
// Handles cross-day scenarios (e.g., 23:00 - 02:00)
func (s *FreezeScheduleService) isTimeInWindow(current, from, to string) bool {
	currentMinutes := s.timeToMinutes(current)
	fromMinutes := s.timeToMinutes(from)
	toMinutes := s.timeToMinutes(to)

	if fromMinutes <= toMinutes {
		// Same day window (e.g., 09:00 - 17:00)
		return currentMinutes >= fromMinutes && currentMinutes <= toMinutes
	} else {
		// Cross-day window (e.g., 23:00 - 02:00)
		return currentMinutes >= fromMinutes || currentMinutes <= toMinutes
	}
}

// timeToMinutes converts HH:MM format to minutes since midnight
func (s *FreezeScheduleService) timeToMinutes(timeStr string) int {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])

	return hours*60 + minutes
}

// formatWeekdays formats weekday numbers to readable string
func (s *FreezeScheduleService) formatWeekdays(weekdays []int) string {
	dayNames := map[int]string{
		1: "Mon", 2: "Tue", 3: "Wed", 4: "Thu",
		5: "Fri", 6: "Sat", 7: "Sun",
	}

	var names []string
	for _, day := range weekdays {
		if name, ok := dayNames[day]; ok {
			names = append(names, name)
		}
	}

	return strings.Join(names, ", ")
}

// containsInt checks if an int slice contains a value
func containsInt(slice []int, value int) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
