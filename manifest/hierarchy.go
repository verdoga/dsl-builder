package manifest

// Course — курс с юнитом либо непосредственными lessons и sections.
// Указатели на срезы отличают отсутствие поля от обязательного пустого массива:
// omitempty применяется к указателю, не к длине массива. Указатель на nil-срез
// недопустим, поскольку сериализовался бы как null. Скрытых полей JSON нет.
type Course struct {
	// Title — обязательное непустое название с хотя бы одним непробельным символом.
	Title string `json:"title"`

	// Unit — присутствующий юнит; nil означает отсутствие поля, не null.
	Unit *Unit `json:"unit,omitempty"`

	// Lessons — обязательный массив без юнита; при Unit должен отсутствовать.
	Lessons *[]Entry `json:"lessons,omitempty"`

	// Sections — обязательный массив без юнита; при Unit должен отсутствовать.
	Sections *[]Section `json:"sections,omitempty"`
}

// Unit — содержимое юнита; title, lessons и sections всегда присутствуют.
type Unit struct {
	// Title — непустое название с хотя бы одним непробельным символом.
	Title string `json:"title"`

	// Lessons — непосредственные уроки; пустой ненулевой срез допустим.
	Lessons []Entry `json:"lessons"`

	// Sections — секции в переданном порядке; пустой ненулевой срез допустим.
	Sections []Section `json:"sections"`
}

// Section — секция с обязательными title и непустым lessons, без веса секции.
type Section struct {
	// Title — имя по ограничениям схемы без нормализации и изменения регистра.
	Title string `json:"title"`

	// Lessons — хотя бы одна запись урока в порядке, выбранном catalog.
	Lessons []Entry `json:"lessons"`
}
