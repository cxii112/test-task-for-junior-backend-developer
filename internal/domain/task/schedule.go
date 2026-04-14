package task

import "time"

// Schedule value-object для создания расписания задач.
// Поля взаимоисключающие.
type Schedule struct {
	// SparseDates создать задачи только в указанные даты
	SparseDates []time.Time
	// EveryNthDay создать задачу в каждый N-й день
	EveryNthDay *int64
	// EveryNthMonthDay создавать задачу в N-ое число месяца
	EveryNthMonthDay *int64
	// EveryEvenDay создавать задачи в четные числа (true) или нечетные (false)
	EveryEvenDay *bool
}
