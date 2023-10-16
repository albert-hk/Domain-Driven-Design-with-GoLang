package purchase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Rhymond/go-money"
	"github.com/google/uuid"

	coffeeco "coffeeco/internal" // 利用go的重命名能力, 把internal重命名为一个"named"
	"coffeeco/internal/loyalty"
	"coffeeco/internal/payment"
	"coffeeco/internal/store"
)

// 表示一次购买的行为
type Purchase struct {
	id                 uuid.UUID
	Store              store.Store
	ProductsToPurchase []coffeeco.Product
	total              money.Money
	PaymentMeans       payment.Means
	timeOfPurchase     time.Time
	CardToken          *string
}

// 检查购买的行为的合理性 & 分配id & 记录时间等 -> 均为逻辑的操作
func (p *Purchase) validateAndEnrich() error {
	if len(p.ProductsToPurchase) == 0 {
		return errors.New("purchase must consist of at least one product")
	}
	p.total = *money.New(0, "USD")

	for _, v := range p.ProductsToPurchase {
		newTotal, _ := p.total.Add(&v.BasePrice)
		p.total = *newTotal
	}
	if p.total.IsZero() {
		return errors.New("likely mistake; purchase should never be 0. Please validate")
	}

	p.id = uuid.New()
	p.timeOfPurchase = time.Now()

	return nil
}

// 利用go的隐士继承方式生命service
type CardChargeService interface {
	ChargeCard(ctx context.Context, amount money.Money, cardToken string) error
}

// 利用go的隐士继承方式生命service
type StoreService interface {
	GetStoreSpecificDiscount(ctx context.Context, storeID uuid.UUID) (float32, error)
}

// 利用一个struct存储所有的dep的serivce和repo
type Service struct {
	cardService  CardChargeService // 描述付款的逻辑, 使用interface作为service定义
	purchaseRepo Repository        // 描述存储的逻辑, 使用interface作为repo定义
	storeService StoreService      // 用于描述“店铺”的相关逻辑, 使用interface作为service定义
}

func NewService(cardService CardChargeService, purchaseRepo Repository, storeService StoreService) *Service {
	return &Service{cardService: cardService, purchaseRepo: purchaseRepo, storeService: storeService}
}

func (s Service) CompletePurchase(ctx context.Context, storeID uuid.UUID, purchase *Purchase, coffeeBuxCard *loyalty.CoffeeBux) error {
	if err := purchase.validateAndEnrich(); err != nil {
		return err
	}

	if err := s.calculateStoreSpecificDiscount(ctx, storeID, purchase); err != nil {
		return err
	}
	switch purchase.PaymentMeans {
	case payment.MEANS_CARD:
		// 使用service中的用"卡"付款的service处理, 此处为interface
		if err := s.cardService.ChargeCard(ctx, purchase.total, *purchase.CardToken); err != nil {
			return errors.New("card charge failed, cancelling purchase")
		}
	case payment.MEANS_CASH:
	// For the reader to add :)

	case payment.MEANS_COFFEEBUX:
		// 使用传入的用户忠诚计划的信息付款, 注意, 此处非interface
		if err := coffeeBuxCard.Pay(ctx, purchase.ProductsToPurchase); err != nil {
			return fmt.Errorf("failed to charge loyalty card: %w", err)
		}
	default:
		return errors.New("unknown payment type")
	}

	if err := s.purchaseRepo.Store(ctx, *purchase); err != nil {
		return errors.New("failed to Store purchase")
	}
	if coffeeBuxCard != nil {
		coffeeBuxCard.AddStamp()
	}
	return nil
}

func (s *Service) calculateStoreSpecificDiscount(ctx context.Context, storeID uuid.UUID, purchase *Purchase) error {
	discount, err := s.storeService.GetStoreSpecificDiscount(ctx, storeID)
	if err != nil && err != store.ErrNoDiscount {
		return fmt.Errorf("failed to get discount: %w", err)
	}

	purchasePrice := purchase.total
	if discount > 0 {
		purchase.total = *purchasePrice.Multiply(int64(100 - discount))
	}
	return nil
}
