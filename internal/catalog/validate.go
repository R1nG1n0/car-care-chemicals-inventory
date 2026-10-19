package catalog

// Экспортируемые проверки полей для импорта файла (ФТ-10.3.3): файл
// проверяется по тем же правилам, что и ввод через обычный API.

// ValidateProduct проверяет и нормализует поля позиции (категорию не проверяет).
func ValidateProduct(in ProductInput) (ProductInput, error) {
	err := in.validate()
	return in, err
}

// CleanRefName проверяет название категории, места или магазина (1…40 символов).
func CleanRefName(field, name string) (string, error) {
	return cleanName(field, name, refNameMax, "Название")
}

// ValidateColor проверяет цвет категории.
func ValidateColor(c string) error { return validColor(c) }

// ValidateStoreURL проверяет ссылку магазина: пусто или http(s)://…
func ValidateStoreURL(raw string) (string, error) { return validStoreURL(raw) }

// CleanShoppingTitle проверяет текст пункта списка покупок (0…80 символов).
func CleanShoppingTitle(title string) (string, error) {
	return optText("title", title, 80, "Название")
}
