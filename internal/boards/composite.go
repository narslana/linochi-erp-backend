package boards

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/calendar"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/ids"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/inventory"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/kiosk"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/products"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/purchasing"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/shipments"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/suppliers"
)

// Composite implements api.ServerInterface by delegating to auth, boards, products, ids, suppliers, purchasing, shipments, inventory, calendar and kiosk handlers.
// It is mounted once via api.HandlerWithOptions so that routes are registered only once.
type Composite struct {
	auth       *auth.Handler
	boards     *Handler
	products   *products.Handler
	ids        *ids.Handler
	suppliers  *suppliers.Handler
	purchasing *purchasing.Handler
	shipments  *shipments.Handler
	inventory  *inventory.Handler
	calendar   *calendar.Handler
	kiosk      *kiosk.Handler
}

// NewComposite creates a composite handler.
func NewComposite(auth *auth.Handler, boards *Handler, productsH *products.Handler, idsH *ids.Handler, suppliersH *suppliers.Handler, purchasingH *purchasing.Handler, shipmentsH *shipments.Handler, inventoryH *inventory.Handler, calendarH *calendar.Handler, kioskH *kiosk.Handler) *Composite {
	return &Composite{auth: auth, boards: boards, products: productsH, ids: idsH, suppliers: suppliersH, purchasing: purchasingH, shipments: shipmentsH, inventory: inventoryH, calendar: calendarH, kiosk: kioskH}
}

// Mount registers the composite on the given router.
func (c *Composite) Mount(m api.ServeMux) {
	api.HandlerWithOptions(c, api.StdHTTPServerOptions{BaseRouter: m})
}

// ─── Auth routes (delegated) ────────────────────────────────────────────────

func (c *Composite) GetMe(w http.ResponseWriter, r *http.Request)      { c.auth.GetMe(w, r) }
func (c *Composite) GetVersion(w http.ResponseWriter, r *http.Request) { c.auth.GetVersion(w, r) }
func (c *Composite) LoginCallback(w http.ResponseWriter, r *http.Request, params api.LoginCallbackParams) {
	c.auth.LoginCallback(w, r, params)
}
func (c *Composite) Login(w http.ResponseWriter, r *http.Request, params api.LoginParams) {
	c.auth.Login(w, r, params)
}
func (c *Composite) Logout(w http.ResponseWriter, r *http.Request) { c.auth.Logout(w, r) }

// ─── Boards routes (delegated) ─────────────────────────────────────────────

func (c *Composite) ListBoards(w http.ResponseWriter, r *http.Request)  { c.boards.ListBoards(w, r) }
func (c *Composite) CreateBoard(w http.ResponseWriter, r *http.Request) { c.boards.CreateBoard(w, r) }
func (c *Composite) ArchiveBoard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	c.boards.ArchiveBoard(w, r, boardId)
}
func (c *Composite) GetBoard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	c.boards.GetBoard(w, r, boardId)
}
func (c *Composite) UpdateBoard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	c.boards.UpdateBoard(w, r, boardId)
}
func (c *Composite) ListCards(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	c.boards.ListCards(w, r, boardId)
}
func (c *Composite) CreateCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	c.boards.CreateCard(w, r, boardId)
}
func (c *Composite) ArchiveCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	c.boards.ArchiveCard(w, r, boardId, cardId)
}
func (c *Composite) GetCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	c.boards.GetCard(w, r, boardId, cardId)
}
func (c *Composite) UpdateCard(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	c.boards.UpdateCard(w, r, boardId, cardId)
}
func (c *Composite) ListChecklistItems(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	c.boards.ListChecklistItems(w, r, boardId, cardId)
}
func (c *Composite) CreateChecklistItem(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	c.boards.CreateChecklistItem(w, r, boardId, cardId)
}
func (c *Composite) ArchiveChecklistItem(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID, itemId openapi_types.UUID) {
	c.boards.ArchiveChecklistItem(w, r, boardId, cardId, itemId)
}
func (c *Composite) UpdateChecklistItem(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID, itemId openapi_types.UUID) {
	c.boards.UpdateChecklistItem(w, r, boardId, cardId, itemId)
}
func (c *Composite) ListComments(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	c.boards.ListComments(w, r, boardId, cardId)
}
func (c *Composite) CreateComment(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, cardId openapi_types.UUID) {
	c.boards.CreateComment(w, r, boardId, cardId)
}
func (c *Composite) ListColumns(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	c.boards.ListColumns(w, r, boardId)
}
func (c *Composite) CreateColumn(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID) {
	c.boards.CreateColumn(w, r, boardId)
}
func (c *Composite) ArchiveColumn(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, columnId openapi_types.UUID) {
	c.boards.ArchiveColumn(w, r, boardId, columnId)
}
func (c *Composite) UpdateColumn(w http.ResponseWriter, r *http.Request, boardId openapi_types.UUID, columnId openapi_types.UUID) {
	c.boards.UpdateColumn(w, r, boardId, columnId)
}

// ─── Products routes (delegated) ───────────────────────────────────────────

func (c *Composite) ListFieldDefinitions(w http.ResponseWriter, r *http.Request) {
	c.products.ListFieldDefinitions(w, r)
}
func (c *Composite) CreateFieldDefinition(w http.ResponseWriter, r *http.Request) {
	c.products.CreateFieldDefinition(w, r)
}
func (c *Composite) ArchiveFieldDefinition(w http.ResponseWriter, r *http.Request, fieldDefinitionId openapi_types.UUID) {
	c.products.ArchiveFieldDefinition(w, r, fieldDefinitionId)
}
func (c *Composite) GetFieldDefinition(w http.ResponseWriter, r *http.Request, fieldDefinitionId openapi_types.UUID) {
	c.products.GetFieldDefinition(w, r, fieldDefinitionId)
}
func (c *Composite) UpdateFieldDefinition(w http.ResponseWriter, r *http.Request, fieldDefinitionId openapi_types.UUID) {
	c.products.UpdateFieldDefinition(w, r, fieldDefinitionId)
}
func (c *Composite) ListProductTypes(w http.ResponseWriter, r *http.Request) {
	c.products.ListProductTypes(w, r)
}
func (c *Composite) CreateProductType(w http.ResponseWriter, r *http.Request) {
	c.products.CreateProductType(w, r)
}
func (c *Composite) ArchiveProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	c.products.ArchiveProductType(w, r, productTypeId)
}
func (c *Composite) GetProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	c.products.GetProductType(w, r, productTypeId)
}
func (c *Composite) UpdateProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	c.products.UpdateProductType(w, r, productTypeId)
}
func (c *Composite) ListProductTypeFields(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	c.products.ListProductTypeFields(w, r, productTypeId)
}
func (c *Composite) AssignFieldToProductType(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID) {
	c.products.AssignFieldToProductType(w, r, productTypeId)
}
func (c *Composite) ArchiveProductTypeField(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID, ptfId openapi_types.UUID) {
	c.products.ArchiveProductTypeField(w, r, productTypeId, ptfId)
}
func (c *Composite) UpdateProductTypeField(w http.ResponseWriter, r *http.Request, productTypeId openapi_types.UUID, ptfId openapi_types.UUID) {
	c.products.UpdateProductTypeField(w, r, productTypeId, ptfId)
}
func (c *Composite) ListProducts(w http.ResponseWriter, r *http.Request, params api.ListProductsParams) {
	c.products.ListProducts(w, r, params)
}
func (c *Composite) CreateProduct(w http.ResponseWriter, r *http.Request) {
	c.products.CreateProduct(w, r)
}
func (c *Composite) ArchiveProduct(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	c.products.ArchiveProduct(w, r, productId)
}
func (c *Composite) GetProduct(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	c.products.GetProduct(w, r, productId)
}
func (c *Composite) UpdateProduct(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	c.products.UpdateProduct(w, r, productId)
}
func (c *Composite) LockProductSKU(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	c.products.LockProductSKU(w, r, productId)
}
func (c *Composite) ListProductImages(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	c.products.ListProductImages(w, r, productId)
}
func (c *Composite) CreateProductImage(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	c.products.CreateProductImage(w, r, productId)
}
func (c *Composite) ArchiveProductImage(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, imageId openapi_types.UUID) {
	c.products.ArchiveProductImage(w, r, productId, imageId)
}
func (c *Composite) UpdateProductImage(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, imageId openapi_types.UUID) {
	c.products.UpdateProductImage(w, r, productId, imageId)
}
func (c *Composite) ListProductDocuments(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	c.products.ListProductDocuments(w, r, productId)
}
func (c *Composite) CreateProductDocument(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	c.products.CreateProductDocument(w, r, productId)
}
func (c *Composite) ArchiveProductDocument(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, docId openapi_types.UUID) {
	c.products.ArchiveProductDocument(w, r, productId, docId)
}
func (c *Composite) UpdateProductDocument(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, docId openapi_types.UUID) {
	c.products.UpdateProductDocument(w, r, productId, docId)
}

// ─── IDs routes (delegated) ────────────────────────────────────────────────

func (c *Composite) ListIdsEntries(w http.ResponseWriter, r *http.Request) {
	c.ids.ListIdsEntries(w, r)
}
func (c *Composite) CreateIdsEntry(w http.ResponseWriter, r *http.Request) {
	c.ids.CreateIdsEntry(w, r)
}
func (c *Composite) ArchiveIdsEntry(w http.ResponseWriter, r *http.Request, entryId openapi_types.UUID) {
	c.ids.ArchiveIdsEntry(w, r, entryId)
}
func (c *Composite) GetIdsEntry(w http.ResponseWriter, r *http.Request, entryId openapi_types.UUID) {
	c.ids.GetIdsEntry(w, r, entryId)
}
func (c *Composite) UpdateIdsEntry(w http.ResponseWriter, r *http.Request, entryId openapi_types.UUID) {
	c.ids.UpdateIdsEntry(w, r, entryId)
}

// ─── Suppliers routes (delegated) ───────────────────────────────────────────

func (c *Composite) ListSuppliers(w http.ResponseWriter, r *http.Request) {
	c.suppliers.ListSuppliers(w, r)
}
func (c *Composite) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	c.suppliers.CreateSupplier(w, r)
}
func (c *Composite) ArchiveSupplier(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	c.suppliers.ArchiveSupplier(w, r, supplierId)
}
func (c *Composite) GetSupplier(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	c.suppliers.GetSupplier(w, r, supplierId)
}
func (c *Composite) UpdateSupplier(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	c.suppliers.UpdateSupplier(w, r, supplierId)
}
func (c *Composite) ListSupplierContacts(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	c.suppliers.ListSupplierContacts(w, r, supplierId)
}
func (c *Composite) CreateSupplierContact(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	c.suppliers.CreateSupplierContact(w, r, supplierId)
}
func (c *Composite) ArchiveSupplierContact(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID, contactId openapi_types.UUID) {
	c.suppliers.ArchiveSupplierContact(w, r, supplierId, contactId)
}
func (c *Composite) UpdateSupplierContact(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID, contactId openapi_types.UUID) {
	c.suppliers.UpdateSupplierContact(w, r, supplierId, contactId)
}
func (c *Composite) ListSupplierProducts(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	c.suppliers.ListSupplierProducts(w, r, supplierId)
}
func (c *Composite) CreateSupplierProduct(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID) {
	c.suppliers.CreateSupplierProduct(w, r, supplierId)
}
func (c *Composite) ArchiveSupplierProduct(w http.ResponseWriter, r *http.Request, supplierId openapi_types.UUID, spId openapi_types.UUID) {
	c.suppliers.ArchiveSupplierProduct(w, r, supplierId, spId)
}

// ─── Purchasing routes (delegated) ──────────────────────────────────────────

func (c *Composite) ListPurchaseOrders(w http.ResponseWriter, r *http.Request, params api.ListPurchaseOrdersParams) {
	c.purchasing.ListPurchaseOrders(w, r, params)
}
func (c *Composite) CreatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	c.purchasing.CreatePurchaseOrder(w, r)
}
func (c *Composite) ArchivePurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	c.purchasing.ArchivePurchaseOrder(w, r, poId)
}
func (c *Composite) GetPurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	c.purchasing.GetPurchaseOrder(w, r, poId)
}
func (c *Composite) UpdatePurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	c.purchasing.UpdatePurchaseOrder(w, r, poId)
}
func (c *Composite) ListPurchaseOrderLines(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	c.purchasing.ListPurchaseOrderLines(w, r, poId)
}
func (c *Composite) CreatePurchaseOrderLine(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	c.purchasing.CreatePurchaseOrderLine(w, r, poId)
}
func (c *Composite) UpdatePurchaseOrderLine(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID, lineId openapi_types.UUID) {
	c.purchasing.UpdatePurchaseOrderLine(w, r, poId, lineId)
}
func (c *Composite) ArchivePurchaseOrderLine(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID, lineId openapi_types.UUID) {
	c.purchasing.ArchivePurchaseOrderLine(w, r, poId, lineId)
}

// ─── Shipments routes (delegated) ───────────────────────────────────────────

func (c *Composite) ListShipments(w http.ResponseWriter, r *http.Request) {
	c.shipments.ListShipments(w, r)
}
func (c *Composite) CreateShipment(w http.ResponseWriter, r *http.Request) {
	c.shipments.CreateShipment(w, r)
}
func (c *Composite) ArchiveShipment(w http.ResponseWriter, r *http.Request, shipmentId openapi_types.UUID) {
	c.shipments.ArchiveShipment(w, r, shipmentId)
}
func (c *Composite) GetShipment(w http.ResponseWriter, r *http.Request, shipmentId openapi_types.UUID) {
	c.shipments.GetShipment(w, r, shipmentId)
}
func (c *Composite) UpdateShipment(w http.ResponseWriter, r *http.Request, shipmentId openapi_types.UUID) {
	c.shipments.UpdateShipment(w, r, shipmentId)
}
func (c *Composite) ListShipmentsByPurchaseOrder(w http.ResponseWriter, r *http.Request, poId openapi_types.UUID) {
	c.shipments.ListShipmentsByPurchaseOrder(w, r, poId)
}

// ─── Inventory routes (delegated) ────────────────────────────────────────────

func (c *Composite) ListLocations(w http.ResponseWriter, r *http.Request) {
	c.inventory.ListLocations(w, r)
}
func (c *Composite) CreateLocation(w http.ResponseWriter, r *http.Request) {
	c.inventory.CreateLocation(w, r)
}
func (c *Composite) ArchiveLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	c.inventory.ArchiveLocation(w, r, locationId)
}
func (c *Composite) GetLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	c.inventory.GetLocation(w, r, locationId)
}
func (c *Composite) UpdateLocation(w http.ResponseWriter, r *http.Request, locationId openapi_types.UUID) {
	c.inventory.UpdateLocation(w, r, locationId)
}
func (c *Composite) ListStockMovements(w http.ResponseWriter, r *http.Request, params api.ListStockMovementsParams) {
	c.inventory.ListStockMovements(w, r, params)
}
func (c *Composite) CreateStockMovement(w http.ResponseWriter, r *http.Request) {
	c.inventory.CreateStockMovement(w, r)
}
func (c *Composite) GetBalance(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID) {
	c.inventory.GetBalance(w, r, productId)
}
func (c *Composite) GetFEFOBatches(w http.ResponseWriter, r *http.Request, productId openapi_types.UUID, locationId openapi_types.UUID) {
	c.inventory.GetFEFOBatches(w, r, productId, locationId)
}
func (c *Composite) GetInventoryValue(w http.ResponseWriter, r *http.Request) {
	c.inventory.GetInventoryValue(w, r)
}
func (c *Composite) ReserveStock(w http.ResponseWriter, r *http.Request) {
	c.inventory.ReserveStock(w, r)
}
func (c *Composite) ReleaseStock(w http.ResponseWriter, r *http.Request) {
	c.inventory.ReleaseStock(w, r)
}
func (c *Composite) WithdrawStock(w http.ResponseWriter, r *http.Request) {
	c.inventory.WithdrawStock(w, r)
}

// ─── Calendar routes (delegated) ─────────────────────────────────────────────

func (c *Composite) ListICalSources(w http.ResponseWriter, r *http.Request) {
	c.calendar.ListICalSources(w, r)
}
func (c *Composite) CreateICalSource(w http.ResponseWriter, r *http.Request) {
	c.calendar.CreateICalSource(w, r)
}
func (c *Composite) GetICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID) {
	c.calendar.GetICalSource(w, r, sourceId)
}
func (c *Composite) UpdateICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID) {
	c.calendar.UpdateICalSource(w, r, sourceId)
}
func (c *Composite) ArchiveICalSource(w http.ResponseWriter, r *http.Request, sourceId openapi_types.UUID, params api.ArchiveICalSourceParams) {
	c.calendar.ArchiveICalSource(w, r, sourceId, params)
}
func (c *Composite) ListCalendarEvents(w http.ResponseWriter, r *http.Request, params api.ListCalendarEventsParams) {
	c.calendar.ListCalendarEvents(w, r, params)
}
func (c *Composite) GetCalendarEvent(w http.ResponseWriter, r *http.Request, eventId openapi_types.UUID) {
	c.calendar.GetCalendarEvent(w, r, eventId)
}
func (c *Composite) SyncCalendar(w http.ResponseWriter, r *http.Request) {
	c.calendar.SyncCalendar(w, r)
}

// ─── Kiosk routes (delegated) ────────────────────────────────────────────────

func (c *Composite) ListKioskPlaylists(w http.ResponseWriter, r *http.Request) {
	c.kiosk.ListKioskPlaylists(w, r)
}
func (c *Composite) CreateKioskPlaylist(w http.ResponseWriter, r *http.Request) {
	c.kiosk.CreateKioskPlaylist(w, r)
}
func (c *Composite) GetKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	c.kiosk.GetKioskPlaylist(w, r, playlistId)
}
func (c *Composite) UpdateKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	c.kiosk.UpdateKioskPlaylist(w, r, playlistId)
}
func (c *Composite) ArchiveKioskPlaylist(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, params api.ArchiveKioskPlaylistParams) {
	c.kiosk.ArchiveKioskPlaylist(w, r, playlistId, params)
}
func (c *Composite) ListPlaylistViews(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	c.kiosk.ListPlaylistViews(w, r, playlistId)
}
func (c *Composite) CreatePlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID) {
	c.kiosk.CreatePlaylistView(w, r, playlistId)
}
func (c *Composite) UpdatePlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, viewId openapi_types.UUID) {
	c.kiosk.UpdatePlaylistView(w, r, playlistId, viewId)
}
func (c *Composite) ArchivePlaylistView(w http.ResponseWriter, r *http.Request, playlistId openapi_types.UUID, viewId openapi_types.UUID, params api.ArchivePlaylistViewParams) {
	c.kiosk.ArchivePlaylistView(w, r, playlistId, viewId, params)
}
func (c *Composite) ListKioskDevices(w http.ResponseWriter, r *http.Request) {
	c.kiosk.ListKioskDevices(w, r)
}
func (c *Composite) CreateKioskDevice(w http.ResponseWriter, r *http.Request) {
	c.kiosk.CreateKioskDevice(w, r)
}
func (c *Composite) GetKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID) {
	c.kiosk.GetKioskDevice(w, r, deviceId)
}
func (c *Composite) UpdateKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID) {
	c.kiosk.UpdateKioskDevice(w, r, deviceId)
}
func (c *Composite) ArchiveKioskDevice(w http.ResponseWriter, r *http.Request, deviceId openapi_types.UUID, params api.ArchiveKioskDeviceParams) {
	c.kiosk.ArchiveKioskDevice(w, r, deviceId, params)
}
func (c *Composite) PairKioskDevice(w http.ResponseWriter, r *http.Request, params api.PairKioskDeviceParams) {
	c.kiosk.PairKioskDevice(w, r, params)
}
func (c *Composite) GetKioskData(w http.ResponseWriter, r *http.Request, params api.GetKioskDataParams) {
	c.kiosk.GetKioskData(w, r, params)
}
