package coffeeco // internal 下的pkg名字叫做coffeeco

import "github.com/Rhymond/go-money"

type Product struct {
	ItemName  string
	BasePrice money.Money
}
