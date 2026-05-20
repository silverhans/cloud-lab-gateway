package templates

// Course is a fake Moodle course rendered by the emulator.
type Course struct {
	ExternalID string
	Title      string
	Labs       []Lab
}

// Lab is a fake Moodle lab resource link.
type Lab struct {
	Slug  string
	Title string
}

// User is a fake Moodle user.
type User struct {
	ExternalID string
	Name       string
	Role       string
}

var Courses = []Course{
	{
		ExternalID: "linux-101",
		Title:      "Linux основы",
		Labs: []Lab{
			{Slug: "linux-basics-1", Title: "Лаба 1: пользователи и права"},
			{Slug: "linux-basics-2", Title: "Лаба 2: cron и systemd"},
		},
	},
	{
		ExternalID: "nginx-201",
		Title:      "Nginx и конфигурация",
		Labs: []Lab{
			{Slug: "nginx-config-1", Title: "Лаба 1: SSL + reverse proxy"},
		},
	},
}

var Users = []User{
	{ExternalID: "student-001", Name: "Иван Петров", Role: "Learner"},
	{ExternalID: "student-002", Name: "Анна Смирнова", Role: "Learner"},
	{ExternalID: "student-003", Name: "Олег Иванов", Role: "Learner"},
	{ExternalID: "student-004", Name: "Мария Кузнецова", Role: "Learner"},
	{ExternalID: "student-005", Name: "Дмитрий Соколов", Role: "Learner"},
	{ExternalID: "teacher-001", Name: "Преподаватель Сергей", Role: "Instructor"},
}
