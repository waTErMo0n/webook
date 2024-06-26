// Copyright 2023 ecodeclub
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build e2e

package integration

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/ecodeclub/ekit/iox"
	"github.com/ecodeclub/ginx/session"
	"github.com/ecodeclub/webook/internal/credit"
	creditmocks "github.com/ecodeclub/webook/internal/credit/mocks"
	"github.com/ecodeclub/webook/internal/order/internal/domain"
	"github.com/ecodeclub/webook/internal/order/internal/errs"
	"github.com/ecodeclub/webook/internal/order/internal/integration/startup"
	"github.com/ecodeclub/webook/internal/order/internal/repository/dao"
	"github.com/ecodeclub/webook/internal/order/internal/web"
	"github.com/ecodeclub/webook/internal/payment"
	"github.com/ecodeclub/webook/internal/product"
	"github.com/ecodeclub/webook/internal/test"
	testioc "github.com/ecodeclub/webook/internal/test/ioc"
	"github.com/ego-component/egorm"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/econf"
	"github.com/gotomicro/ego/server/egin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

const (
	testUID = int64(234)
)

type fakePaymentService struct {
	counter atomic.Int64
}

func (f *fakePaymentService) CreatePayment(ctx context.Context, p payment.Payment) (payment.Payment, error) {
	f.counter.Add(1)
	id := f.counter.Load()

	columns := map[int64]payment.Payment{
		1: {
			ID:          1,
			SN:          "PaymentSN-1",
			OrderID:     p.OrderID,
			OrderSN:     p.OrderSN,
			TotalAmount: p.TotalAmount,
			PayDDL:      p.PayDDL,
			Records: []payment.Record{
				{
					PaymentNO3rd: "credit-1",
					Channel:      payment.ChannelTypeCredit,
					Amount:       990,
					Status:       0,
				},
			},
		},
		2: {
			ID:          2,
			SN:          "PaymentSN-2",
			OrderID:     p.OrderID,
			OrderSN:     p.OrderSN,
			TotalAmount: p.TotalAmount,
			PayDDL:      p.PayDDL,
			Records: []payment.Record{
				{
					PaymentNO3rd: "credit-1",
					Channel:      payment.ChannelTypeCredit,
					Amount:       1000,
					Status:       0,
				},
				{
					PaymentNO3rd:  "wechat-2",
					Channel:       payment.ChannelTypeWechat,
					Amount:        8990,
					Status:        0,
					WechatCodeURL: "webchat_code",
				},
			},
		},
	}
	r, ok := columns[id]
	if !ok {
		return payment.Payment{}, fmt.Errorf(fmt.Sprintf("未配置的支付id=%d", id))
	}
	return r, nil
}

func (f *fakePaymentService) GetPaymentChannels(ctx context.Context) []payment.Channel {
	return []payment.Channel{
		{Type: 1, Desc: "积分"},
		{Type: 2, Desc: "微信"},
	}
}

func (f *fakePaymentService) FindPaymentByID(ctx context.Context, paymentID int64) (payment.Payment, error) {
	payments := map[int64]payment.Payment{
		33: {
			ID:      33,
			SN:      "paymentSN-33",
			OrderID: 0,
			OrderSN: "orderSN-33",
			Records: []payment.Record{
				{
					Channel: payment.ChannelTypeCredit,
					Amount:  9900,
				},
			},
		},
	}
	p, ok := payments[paymentID]
	if !ok {
		return payment.Payment{}, fmt.Errorf(fmt.Sprintf("未配置的支付ID = %d", paymentID))
	}
	return p, nil
}

type fakeProductService struct{}

func (f *fakeProductService) FindBySN(_ context.Context, sn string) (product.Product, error) {
	var StatusOnShelf int64 = 2
	products := map[string]product.Product{
		"SKU100": {
			SPU: product.SPU{
				ID:     100,
				SN:     "SPUSN100",
				Name:   "商品SPU100",
				Desc:   "商品SPU100描述",
				Status: StatusOnShelf,
			},
			SKU: product.SKU{
				ID:       100,
				SN:       "SKU100",
				Name:     "商品SKU100",
				Desc:     "商品SKU100",
				Price:    990,
				Stock:    10,
				SaleType: 1, // 无限制
				Status:   StatusOnShelf,
			},
		},
		"SKU101": {
			SPU: product.SPU{
				ID:     101,
				SN:     "SPUSN101",
				Name:   "商品SPU101",
				Desc:   "商品SPU101描述",
				Status: StatusOnShelf,
			},
			SKU: product.SKU{
				ID:       101,
				SN:       "SKU101",
				Name:     "商品SKU101",
				Desc:     "商品SKU101",
				Price:    9900,
				Stock:    1,
				SaleType: 1, // 无限制
				Status:   StatusOnShelf,
			},
		},
	}

	if _, ok := products[sn]; !ok {
		return product.Product{}, fmt.Errorf(fmt.Sprintf("fakeProductService未配置的SN=%s", sn))
	}

	return products[sn], nil
}

type HandlerTestSuite struct {
	suite.Suite
	server *egin.Component
	db     *egorm.Component
	dao    dao.OrderDAO
	ctrl   *gomock.Controller
}

func (s *HandlerTestSuite) SetupSuite() {

	s.ctrl = gomock.NewController(s.T())

	mockedCreditSvc := creditmocks.NewMockService(s.ctrl)
	mockedCreditSvc.EXPECT().GetCreditsByUID(gomock.Any(), testUID).AnyTimes().Return(credit.Credit{
		TotalAmount: 1000,
	}, nil)

	handler, err := startup.InitHandler(&fakePaymentService{}, &fakeProductService{}, mockedCreditSvc)
	require.NoError(s.T(), err)

	econf.Set("server", map[string]any{"contextTimeout": "1s"})
	server := egin.Load("server").Build()
	server.Use(func(ctx *gin.Context) {
		ctx.Set("_session", session.NewMemorySession(session.Claims{
			Uid: testUID,
		}))
	})
	handler.PrivateRoutes(server.Engine)

	s.server = server
	s.db = testioc.InitDB()
	err = dao.InitTables(s.db)
	require.NoError(s.T(), err)
	s.dao = dao.NewOrderGORMDAO(s.db)
}

func (s *HandlerTestSuite) TearDownSuite() {
	err := s.db.Exec("DROP TABLE `orders`").Error
	require.NoError(s.T(), err)
	err = s.db.Exec("DROP TABLE `order_items`").Error
	require.NoError(s.T(), err)

	s.ctrl.Finish()
}

func (s *HandlerTestSuite) TearDownTest() {
	err := s.db.Exec("TRUNCATE TABLE `orders`").Error
	require.NoError(s.T(), err)
	err = s.db.Exec("TRUNCATE TABLE `order_items`").Error
	require.NoError(s.T(), err)
}

func (s *HandlerTestSuite) TestPreviewOrder() {

	testCases := []struct {
		name string

		req      web.PreviewOrderReq
		wantCode int
		wantResp test.Result[web.PreviewOrderResp]
	}{
		{
			name: "获取成功",
			req: web.PreviewOrderReq{
				ProductSKUSN: "SKU100",
				Quantity:     1,
			},
			wantCode: 200,
			wantResp: test.Result[web.PreviewOrderResp]{
				Data: web.PreviewOrderResp{
					Credits: 1000,
					Payments: []web.Payment{
						{Type: payment.ChannelTypeCredit},
						{Type: payment.ChannelTypeWechat},
					},
					Products: []web.Product{
						{
							SPUSN:         "SPUSN100",
							SKUSN:         "SKU100",
							Name:          "商品SKU100",
							Desc:          "商品SKU100",
							OriginalPrice: 990,
							RealPrice:     990,
							Quantity:      1,
						},
					},
					Policy: "请注意: 虚拟商品、一旦支持成功不退、不换,请谨慎操作",
				},
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/order/preview", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[web.PreviewOrderResp]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantResp, recorder.MustScan())
		})
	}
}

func (s *HandlerTestSuite) TestPreviewOrderFailed() {
	testCases := []struct {
		name string

		req      web.PreviewOrderReq
		wantCode int
		wantResp test.Result[any]
	}{
		{
			name: "商品SKUSN不存在",
			req: web.PreviewOrderReq{
				ProductSKUSN: "InvalidSN",
				Quantity:     1,
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "要购买的商品数量非法",
			req: web.PreviewOrderReq{
				ProductSKUSN: "SKU100",
				Quantity:     0,
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "商品库存不足",
			req: web.PreviewOrderReq{
				ProductSKUSN: "SKU100",
				Quantity:     11,
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		// todo: 要购买商品超过库存限制(stockLimit)但是库存充足
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/order/preview", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[any]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantResp, recorder.MustScan())
		})
	}
}

func (s *HandlerTestSuite) TestCreateOrderAndPayment() {
	var testCases = []struct {
		name           string
		req            web.CreateOrderReq
		wantCode       int
		assertRespFunc func(t *testing.T, resp test.Result[web.CreateOrderResp])
	}{
		{
			name: "创建成功_仅积分支付",
			req: web.CreateOrderReq{
				RequestID: "requestID01",
				Products: []web.Product{
					{
						SKUSN:    "SKU100",
						Quantity: 1,
					},
				},
				Payments: []web.Payment{
					{Type: payment.ChannelTypeCredit},
					{Type: payment.ChannelTypeWechat},
				},
				OriginalTotalPrice: 990,
				RealTotalPrice:     990,
			},
			wantCode: 200,
			assertRespFunc: func(t *testing.T, result test.Result[web.CreateOrderResp]) {
				t.Helper()
				assert.NotZero(t, result.Data.OrderSN)
				assert.Zero(t, result.Data.WechatCodeURL)
			},
		},
		// todo: 创建成功_仅微信支付
		{
			name: "创建成功_积分和微信组合支付",
			req: web.CreateOrderReq{
				RequestID: "requestID02",
				Products: []web.Product{
					{
						SKUSN:    "SKU101",
						Quantity: 1,
					},
				},
				Payments: []web.Payment{
					{Type: payment.ChannelTypeCredit},
					{Type: payment.ChannelTypeWechat},
				},
				OriginalTotalPrice: 9900,
				RealTotalPrice:     9900,
			},
			wantCode: 200,
			assertRespFunc: func(t *testing.T, result test.Result[web.CreateOrderResp]) {
				t.Helper()
				assert.NotZero(t, result.Data.OrderSN)
				assert.NotZero(t, result.Data.WechatCodeURL)
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/order/create", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[web.CreateOrderResp]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			tc.assertRespFunc(t, recorder.MustScan())
		})
	}
}

func (s *HandlerTestSuite) TestCreateOrderAndPaymentFailed() {
	testCases := []struct {
		name string

		req      web.CreateOrderReq
		wantCode int
		wantResp test.Result[any]
	}{
		{
			name: "请求ID为空",
			req: web.CreateOrderReq{
				RequestID: "",
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "商品信息非法",
			req: web.CreateOrderReq{
				RequestID: "requestID09",
				Products:  []web.Product{},
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "商品SKUSN不存在",
			req: web.CreateOrderReq{
				RequestID: "requestID03",
				Products: []web.Product{
					{
						SKUSN: "InvalidSKUSN",
					},
				},
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "要购买的商品数量非法",
			req: web.CreateOrderReq{
				RequestID: "requestID04",
				Products: []web.Product{
					{
						SKUSN:    "SKU100",
						Quantity: 0,
					},
				},
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "商品库存不足",
			req: web.CreateOrderReq{
				RequestID: "requestID05",
				Products: []web.Product{
					{
						SKUSN:    "SKU100",
						Quantity: 11,
					},
				},
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "商品总原价非法",
			req: web.CreateOrderReq{
				RequestID: "requestID06",
				Products: []web.Product{
					{
						SKUSN:    "SKU100",
						Quantity: 10,
					},
				},
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "商品总实价非法",
			req: web.CreateOrderReq{
				RequestID: "requestID07",
				Products: []web.Product{
					{
						SKUSN:    "SKU100",
						Quantity: 10,
					},
				},
				OriginalTotalPrice: 10 * 990,
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "支付渠道非法",
			req: web.CreateOrderReq{
				RequestID: "requestID08",
				Products: []web.Product{
					{
						SKUSN:    "SKU100",
						Quantity: 10,
					},
				},
				Payments: []web.Payment{
					{
						Type: 0,
					},
				},
				OriginalTotalPrice: 10 * 990,
				RealTotalPrice:     10 * 990,
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "请求重复",
			req: web.CreateOrderReq{
				RequestID: "requestID08",
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		// todo: 要购买商品超过库存限制(stockLimit)但是库存充足
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/order/create", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[any]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantResp, recorder.MustScan())
		})
	}
}

func (s *HandlerTestSuite) TestRetrieveOrderStatus() {
	var testCases = []struct {
		name string

		before         func(t *testing.T)
		req            web.RetrieveOrderStatusReq
		wantCode       int
		assertRespFunc func(t *testing.T, result test.Result[web.RetrieveOrderStatusResp])
	}{
		{
			name: "获取订单状态成功",
			before: func(t *testing.T) {
				t.Helper()
				_, err := s.dao.CreateOrder(context.Background(), dao.Order{
					SN:        "orderSN-1",
					BuyerId:   testUID,
					PaymentId: 12,
					PaymentSn: "paymentSN-12",
				}, []dao.OrderItem{
					{
						Id:               0,
						OrderId:          0,
						SPUId:            1,
						SKUId:            1,
						SKUName:          "商品SKU",
						SKUDescription:   "商品SKU描述",
						SKUOriginalPrice: 9900,
						SKURealPrice:     9900,
						Quantity:         1,
					},
				})
				require.NoError(t, err)
			},

			req: web.RetrieveOrderStatusReq{
				OrderSN: "orderSN-1",
			},
			wantCode: 200,
			assertRespFunc: func(t *testing.T, result test.Result[web.RetrieveOrderStatusResp]) {
				t.Helper()
				assert.NotZero(t, result.Data.OrderStatus)
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			tc.before(t)
			req, err := http.NewRequest(http.MethodPost,
				"/order", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[web.RetrieveOrderStatusResp]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			tc.assertRespFunc(t, recorder.MustScan())
		})
	}
}

func (s *HandlerTestSuite) TestRetrieveOrderStatusFailed() {
	testCases := []struct {
		name     string
		req      web.RetrieveOrderStatusReq
		wantCode int
		wantResp test.Result[any]
	}{
		{
			name: "订单序列号为空",
			req: web.RetrieveOrderStatusReq{
				OrderSN: "",
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "订单序列号非法",
			req: web.RetrieveOrderStatusReq{
				OrderSN: "InvalidOrderSN",
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/order", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[any]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantResp, recorder.MustScan())
		})
	}
}

func (s *HandlerTestSuite) TestCompleteOrder() {
	var testCases = []struct {
		name string

		before   func(t *testing.T)
		after    func(t *testing.T)
		req      web.CompleteOrderReq
		wantCode int
		wantResp test.Result[any]
	}{
		{
			name: "完成订单成功",
			before: func(t *testing.T) {
				t.Helper()
				_, err := s.dao.CreateOrder(context.Background(), dao.Order{
					SN:        "orderSN-22",
					BuyerId:   testUID,
					PaymentId: 22,
					PaymentSn: "paymentSN-22",
				}, []dao.OrderItem{
					{
						SPUId:            1,
						SKUId:            1,
						SKUName:          "商品SKU",
						SKUDescription:   "商品SKU描述",
						SKUOriginalPrice: 9900,
						SKURealPrice:     9900,
						Quantity:         1,
					},
				})
				require.NoError(t, err)
			},
			after: func(t *testing.T) {
				t.Helper()
				order, err := s.dao.FindOrderBySNAndBuyerID(context.Background(), "orderSN-22", testUID)
				assert.NoError(t, err)
				assert.Equal(t, int64(domain.OrderStatusCompleted), order.Status)
			},
			req: web.CompleteOrderReq{
				OrderSN: "orderSN-22",
				BuyerID: testUID,
			},
			wantCode: 200,
			wantResp: test.Result[any]{
				Msg: "OK",
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			tc.before(t)
			req, err := http.NewRequest(http.MethodPost,
				"/order/complete", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[web.RetrieveOrderDetailResp]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			tc.after(t)
		})
	}
}

func (s *HandlerTestSuite) TestCompleteOrderFailed() {
	testCases := []struct {
		name     string
		req      web.CompleteOrderReq
		wantCode int
		wantResp test.Result[any]
	}{
		{
			name: "订单序列号为空",
			req: web.CompleteOrderReq{
				OrderSN: "",
				BuyerID: testUID,
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "订单序列号非法",
			req: web.CompleteOrderReq{
				OrderSN: "InvalidOrderSN",
				BuyerID: testUID,
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "买家ID非法",
			req: web.CompleteOrderReq{
				OrderSN: "OrderSN-3",
				BuyerID: 0,
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/order/complete", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[any]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantResp, recorder.MustScan())
		})
	}
}

func (s *HandlerTestSuite) TestCloseTimeoutOrders() {

	total := 15

	testCases := []struct {
		name string
		req  web.CloseTimeoutOrdersReq

		before   func(t *testing.T)
		after    func(t *testing.T)
		wantCode int
		wantResp test.Result[any]
	}{
		{
			name: "关闭超时订单成功_正常情况",
			before: func(t *testing.T) {
				t.Helper()
				for idx := 0; idx < total; idx++ {
					id := int64(200 + idx)
					order := dao.Order{
						Id:                 id,
						SN:                 fmt.Sprintf("OrderSN-close-%d", id),
						PaymentId:          id,
						PaymentSn:          fmt.Sprintf("PaymentSN-close-%d", id),
						BuyerId:            id,
						OriginalTotalPrice: 100,
						RealTotalPrice:     100,
					}
					items := []dao.OrderItem{
						{
							SPUId:            id,
							SKUId:            id,
							SKUName:          fmt.Sprintf("SKUName-%d", id),
							SKUDescription:   fmt.Sprintf("SKUDescription-%d", id),
							SKUOriginalPrice: 100,
							SKURealPrice:     100,
							Quantity:         1,
						},
					}
					_, err := s.dao.CreateOrder(context.Background(), order, items)
					require.NoError(s.T(), err)
				}
			},
			after: func(t *testing.T) {
				t.Helper()
				for idx := 0; idx < total; idx++ {
					id := int64(200 + idx)
					order, err := s.dao.FindOrderBySN(context.Background(), fmt.Sprintf("OrderSN-close-%d", id))
					assert.NoError(t, err)
					assert.Equal(t, int64(domain.OrderStatusExpired), order.Status)
				}
			},
			req: web.CloseTimeoutOrdersReq{
				Limit:  10,
				Minute: 0,
			},
			wantCode: 200,
			wantResp: test.Result[any]{
				Msg: "OK",
			},
		},
		{
			name: "关闭超时订单成功_边界情况",
			before: func(t *testing.T) {
				t.Helper()
				for idx := 0; idx < total; idx++ {
					id := int64(300 + idx)
					order := dao.Order{
						Id:                 id,
						SN:                 fmt.Sprintf("OrderSN-close-%d", id),
						PaymentId:          id,
						PaymentSn:          fmt.Sprintf("PaymentSN-close-%d", id),
						BuyerId:            id,
						OriginalTotalPrice: 100,
						RealTotalPrice:     100,
					}
					items := []dao.OrderItem{
						{
							SPUId:            id,
							SKUId:            id,
							SKUName:          fmt.Sprintf("SKUName-%d", id),
							SKUDescription:   fmt.Sprintf("SKUDescription-%d", id),
							SKUOriginalPrice: 100,
							SKURealPrice:     100,
							Quantity:         1,
						},
					}
					_, err := s.dao.CreateOrder(context.Background(), order, items)
					require.NoError(s.T(), err)
				}
			},
			after: func(t *testing.T) {
				t.Helper()
				for idx := 0; idx < total; idx++ {
					id := int64(300 + idx)
					order, err := s.dao.FindOrderBySN(context.Background(), fmt.Sprintf("OrderSN-close-%d", id))
					assert.NoError(t, err)
					assert.Equal(t, int64(domain.OrderStatusExpired), order.Status)
				}
			},
			req: web.CloseTimeoutOrdersReq{
				Limit:  total,
				Minute: 0,
			},
			wantCode: 200,
			wantResp: test.Result[any]{
				Msg: "OK",
			},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			tc.before(t)
			req, err := http.NewRequest(http.MethodPost,
				"/order/close", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[any]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			require.Equal(t, tc.wantResp, recorder.MustScan())
			tc.after(t)
		})
	}
}

func (s *HandlerTestSuite) TestListOrders() {

	total := 100
	for idx := 0; idx < total; idx++ {
		id := int64(100 + idx)
		order := dao.Order{
			Id:                 id,
			SN:                 fmt.Sprintf("OrderSN-list-%d", id),
			PaymentId:          id,
			PaymentSn:          fmt.Sprintf("PaymentSN-list-%d", id),
			BuyerId:            testUID,
			OriginalTotalPrice: 100,
			RealTotalPrice:     100,
		}
		items := []dao.OrderItem{
			{
				SPUId:            id,
				SKUId:            id,
				SKUName:          fmt.Sprintf("SKUName-%d", id),
				SKUDescription:   fmt.Sprintf("SKUDescription-%d", id),
				SKUOriginalPrice: 100,
				SKURealPrice:     100,
				Quantity:         1,
			},
		}
		_, err := s.dao.CreateOrder(context.Background(), order, items)
		require.NoError(s.T(), err)
	}

	testCases := []struct {
		name string
		req  web.ListOrdersReq

		wantCode int
		wantResp test.Result[web.ListOrdersResp]
	}{
		{
			name: "获取成功",
			req: web.ListOrdersReq{
				Limit:  2,
				Offset: 0,
			},
			wantCode: 200,
			wantResp: test.Result[web.ListOrdersResp]{
				Data: web.ListOrdersResp{
					Total: int64(total),
					Orders: []web.Order{
						{
							SN:                 "OrderSN-list-199",
							PaymentSN:          fmt.Sprintf("PaymentSN-list-%d", 199),
							OriginalTotalPrice: 100,
							RealTotalPrice:     100,
							Status:             domain.OrderStatusUnpaid,
							Items: []web.OrderItem{
								{
									SPUID:            int64(199),
									SKUID:            int64(199),
									SKUName:          fmt.Sprintf("SKUName-%d", 199),
									SKUDescription:   fmt.Sprintf("SKUDescription-%d", 199),
									SKUOriginalPrice: 100,
									SKURealPrice:     100,
									Quantity:         1,
								},
							},
						},
						{
							SN:                 "OrderSN-list-198",
							PaymentSN:          fmt.Sprintf("PaymentSN-list-%d", 198),
							OriginalTotalPrice: 100,
							RealTotalPrice:     100,
							Status:             domain.OrderStatusUnpaid,
							Items: []web.OrderItem{
								{
									SPUID:            int64(198),
									SKUID:            int64(198),
									SKUName:          fmt.Sprintf("SKUName-%d", 198),
									SKUDescription:   fmt.Sprintf("SKUDescription-%d", 198),
									SKUOriginalPrice: 100,
									SKURealPrice:     100,
									Quantity:         1,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "获取部分",
			req: web.ListOrdersReq{
				Limit:  2,
				Offset: 99,
			},
			wantCode: 200,
			wantResp: test.Result[web.ListOrdersResp]{
				Data: web.ListOrdersResp{
					Total: int64(total),
					Orders: []web.Order{
						{
							SN:                 "OrderSN-list-100",
							PaymentSN:          fmt.Sprintf("PaymentSN-list-%d", 100),
							OriginalTotalPrice: 100,
							RealTotalPrice:     100,
							Status:             domain.OrderStatusUnpaid,
							Items: []web.OrderItem{
								{
									SPUID:            int64(100),
									SKUID:            int64(100),
									SKUName:          fmt.Sprintf("SKUName-%d", 100),
									SKUDescription:   fmt.Sprintf("SKUDescription-%d", 100),
									SKUOriginalPrice: 100,
									SKURealPrice:     100,
									Quantity:         1,
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/order/list", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[web.ListOrdersResp]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			s.assertListOrdersRespEqual(t, tc.wantResp.Data, recorder.MustScan().Data)
		})
	}
}

func (s *HandlerTestSuite) assertListOrdersRespEqual(t *testing.T, expected web.ListOrdersResp, actual web.ListOrdersResp) {
	assert.Equal(t, expected.Total, actual.Total)
	assert.Equal(t, len(expected.Orders), len(actual.Orders))
	for i := 0; i < len(actual.Orders); i++ {
		s.assertOrderEqual(t, expected.Orders[i], actual.Orders[i])
	}
}

func (s *HandlerTestSuite) assertOrderEqual(t *testing.T, expected web.Order, actual web.Order) {
	assert.NotZero(t, actual.Ctime)
	assert.NotZero(t, actual.Utime)
	actual.Ctime, actual.Utime = 0, 0
	assert.Equal(t, expected, actual)
}

func (s *HandlerTestSuite) TestRetrieveOrderDetail() {
	var testCases = []struct {
		name string

		before   func(t *testing.T)
		req      web.RetrieveOrderDetailReq
		wantCode int
		wantResp test.Result[web.RetrieveOrderDetailResp]
	}{
		{
			name: "获取订单详情成功",
			before: func(t *testing.T) {
				t.Helper()
				_, err := s.dao.CreateOrder(context.Background(), dao.Order{
					SN:                 "orderSN-33",
					BuyerId:            testUID,
					PaymentId:          33,
					PaymentSn:          "paymentSN-33",
					OriginalTotalPrice: 9900,
					RealTotalPrice:     9900,
				}, []dao.OrderItem{
					{
						SPUId:            1,
						SKUId:            1,
						SKUName:          "商品SKU",
						SKUDescription:   "商品SKU描述",
						SKUOriginalPrice: 9900,
						SKURealPrice:     9900,
						Quantity:         1,
					},
				})
				require.NoError(t, err)
			},

			req: web.RetrieveOrderDetailReq{
				OrderSN: "orderSN-33",
			},
			wantCode: 200,
			wantResp: test.Result[web.RetrieveOrderDetailResp]{
				Data: web.RetrieveOrderDetailResp{
					Order: web.Order{
						SN:                 "orderSN-33",
						PaymentSN:          "paymentSN-33",
						OriginalTotalPrice: 9900,
						RealTotalPrice:     9900,
						Status:             domain.OrderStatusUnpaid,
						Items: []web.OrderItem{
							{
								SPUID:            1,
								SKUID:            1,
								SKUName:          "商品SKU",
								SKUDescription:   "商品SKU描述",
								SKUOriginalPrice: 9900,
								SKURealPrice:     9900,
								Quantity:         1,
							},
						},
						Payments: []web.Payment{
							{
								Type:   payment.ChannelTypeCredit,
								Amount: 9900,
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			tc.before(t)
			req, err := http.NewRequest(http.MethodPost,
				"/order/detail", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[web.RetrieveOrderDetailResp]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			s.assertOrderEqual(t, tc.wantResp.Data.Order, recorder.MustScan().Data.Order)
		})
	}
}

func (s *HandlerTestSuite) TestRetrieveOrderDetailFailed() {
	testCases := []struct {
		name     string
		req      web.RetrieveOrderDetailReq
		wantCode int
		wantResp test.Result[any]
	}{
		{
			name: "订单序列号为空",
			req: web.RetrieveOrderDetailReq{
				OrderSN: "",
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "订单序列号非法",
			req: web.RetrieveOrderDetailReq{
				OrderSN: "InvalidOrderSN",
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/order/detail", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[any]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantResp, recorder.MustScan())
		})
	}
}

func (s *HandlerTestSuite) TestCancelOrder() {
	var (
		testCases = []struct {
			name string

			before   func(t *testing.T)
			after    func(t *testing.T)
			req      web.CancelOrderReq
			wantCode int
			wantResp test.Result[any]
		}{
			{
				name: "取消订单成功",
				before: func(t *testing.T) {
					t.Helper()
					_, err := s.dao.CreateOrder(context.Background(), dao.Order{
						SN:        "orderSN-44",
						BuyerId:   testUID,
						PaymentId: 44,
						PaymentSn: "paymentSN-44",
					}, []dao.OrderItem{
						{
							SPUId:            1,
							SKUId:            1,
							SKUName:          "商品SKU",
							SKUDescription:   "商品SKU描述",
							SKUOriginalPrice: 9900,
							SKURealPrice:     9900,
							Quantity:         1,
						},
					})
					require.NoError(t, err)
				},
				after: func(t *testing.T) {
					t.Helper()
					order, err := s.dao.FindOrderBySNAndBuyerID(context.Background(), "orderSN-44", testUID)
					assert.NoError(t, err)
					assert.Equal(t, int64(domain.OrderStatusCanceled), order.Status)
				},
				req: web.CancelOrderReq{
					OrderSN: "orderSN-44",
				},
				wantCode: 200,
				wantResp: test.Result[any]{
					Msg: "OK",
				},
			},
		}
	)
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			tc.before(t)
			req, err := http.NewRequest(http.MethodPost,
				"/order/cancel", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[web.RetrieveOrderDetailResp]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			tc.after(t)
		})
	}
}

func (s *HandlerTestSuite) TestCancelOrderFailed() {
	testCases := []struct {
		name     string
		req      web.CancelOrderReq
		wantCode int
		wantResp test.Result[any]
	}{
		{
			name: "订单序列号为空",
			req: web.CancelOrderReq{
				OrderSN: "",
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
		{
			name: "订单序列号非法",
			req: web.CancelOrderReq{
				OrderSN: "InvalidOrderSN",
			},
			wantCode: 500,
			wantResp: test.Result[any]{
				Code: errs.SystemError.Code,
				Msg:  errs.SystemError.Msg,
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/order/cancel", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[any]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantResp, recorder.MustScan())
		})
	}
}

func TestHandler(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}
