package database

import (
	"fmt"
	"strconv"

	"github.com/ayushkunwarsingh/forge/server"
)

// RegisterRoutes registers all database HTTP endpoints on the router.
func RegisterRoutes(router *server.Router, engine *Engine) {
	db := router.Group("/db")

	// Collection management
	db.GET("/collections", handleListCollections(engine))
	db.DELETE("/collections/:collection", handleDeleteCollection(engine))

	// Document CRUD
	db.POST("/:collection", handleCreateDocument(engine))
	db.GET("/:collection", handleListDocuments(engine))
	db.GET("/:collection/:id", handleGetDocument(engine))
	db.PUT("/:collection/:id", handleSetDocument(engine))
	db.PATCH("/:collection/:id", handleUpdateDocument(engine))
	db.DELETE("/:collection/:id", handleDeleteDocument(engine))

	// Query endpoint
	db.POST("/_query", handleQuery(engine))

	// Batch write
	db.POST("/_batch", handleBatchWrite(engine))

	// Transaction
	db.POST("/_transaction", handleTransaction(engine))

	// Index management
	db.POST("/_indexes", handleCreateIndex(engine))
	db.GET("/_indexes/:collection", handleListIndexes(engine))

	// Snapshot / Stats
	db.POST("/_snapshot", handleCreateSnapshot(engine))
	db.GET("/_stats", handleStats(engine))
}

// ---- Collection Handlers ----

// handleListCollections handles GET /db/collections
func handleListCollections(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		collections := engine.ListCollections()
		result := make([]map[string]interface{}, 0, len(collections))
		for _, name := range collections {
			result = append(result, map[string]interface{}{
				"name":  name,
				"count": engine.CollectionSize(name),
			})
		}
		ctx.JSON(200, map[string]interface{}{
			"collections": result,
			"total":       len(result),
		})
	}
}

// handleDeleteCollection handles DELETE /db/collections/:collection
func handleDeleteCollection(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		collection := ctx.Param("collection")
		if !engine.DeleteCollection(collection) {
			ctx.Error(404, fmt.Sprintf("Collection '%s' not found", collection))
			return
		}
		ctx.JSON(200, map[string]interface{}{
			"message": fmt.Sprintf("Collection '%s' deleted", collection),
		})
	}
}

// ---- Document CRUD ----

// handleCreateDocument handles POST /db/:collection
func handleCreateDocument(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		collection := ctx.Param("collection")

		var body map[string]interface{}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		// Allow client to specify _id
		docID, _ := body["_id"].(string)
		delete(body, "_id")

		// Remove metadata fields from input
		delete(body, "_created_at")
		delete(body, "_updated_at")
		delete(body, "_version")

		doc, err := engine.SetDocument(collection, docID, body)
		if err != nil {
			ctx.Error(500, "Failed to create document: "+err.Error())
			return
		}

		ctx.JSON(201, map[string]interface{}{
			"document": doc.ToJSON(),
		})
	}
}

// handleGetDocument handles GET /db/:collection/:id
func handleGetDocument(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		collection := ctx.Param("collection")
		docID := ctx.Param("id")

		doc := engine.GetDocument(collection, docID)
		if doc == nil {
			ctx.Error(404, fmt.Sprintf("Document '%s/%s' not found", collection, docID))
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"document": doc.ToJSON(),
		})
	}
}

// handleListDocuments handles GET /db/:collection
func handleListDocuments(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		collection := ctx.Param("collection")

		limit := 100
		offset := 0

		if l := ctx.QueryParam("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
				if limit > 1000 {
					limit = 1000
				}
			}
		}
		if o := ctx.QueryParam("offset"); o != "" {
			if n, err := strconv.Atoi(o); err == nil && n >= 0 {
				offset = n
			}
		}

		docs, total := engine.ListDocuments(collection, limit, offset)

		result := make([]map[string]interface{}, len(docs))
		for i, doc := range docs {
			result[i] = doc.ToJSON()
		}

		ctx.JSON(200, map[string]interface{}{
			"documents": result,
			"count":     len(result),
			"total":     total,
			"limit":     limit,
			"offset":    offset,
		})
	}
}

// handleSetDocument handles PUT /db/:collection/:id (full replace)
func handleSetDocument(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		collection := ctx.Param("collection")
		docID := ctx.Param("id")

		var body map[string]interface{}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		// Remove metadata fields
		delete(body, "_id")
		delete(body, "_created_at")
		delete(body, "_updated_at")
		delete(body, "_version")

		doc, err := engine.SetDocument(collection, docID, body)
		if err != nil {
			ctx.Error(500, "Failed to set document: "+err.Error())
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"document": doc.ToJSON(),
		})
	}
}

// handleUpdateDocument handles PATCH /db/:collection/:id (merge update)
func handleUpdateDocument(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		collection := ctx.Param("collection")
		docID := ctx.Param("id")

		var body map[string]interface{}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		// Remove metadata fields
		delete(body, "_id")
		delete(body, "_created_at")
		delete(body, "_updated_at")
		delete(body, "_version")

		doc, err := engine.UpdateDocument(collection, docID, body)
		if err != nil {
			ctx.Error(404, "Document not found")
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"document": doc.ToJSON(),
		})
	}
}

// handleDeleteDocument handles DELETE /db/:collection/:id
func handleDeleteDocument(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		collection := ctx.Param("collection")
		docID := ctx.Param("id")

		if err := engine.DeleteDocument(collection, docID); err != nil {
			ctx.Error(404, "Document not found")
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"message": "Document deleted",
		})
	}
}

// ---- Query Handler ----

// handleQuery handles POST /db/_query
func handleQuery(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		var body map[string]interface{}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		query, err := ParseQuery(body)
		if err != nil {
			ctx.Error(400, "Invalid query: "+err.Error())
			return
		}

		result, err := engine.ExecuteQuery(query)
		if err != nil {
			ctx.Error(500, "Query execution failed: "+err.Error())
			return
		}

		docs := make([]map[string]interface{}, len(result.Documents))
		for i, doc := range result.Documents {
			docs[i] = doc.ToJSON()
		}

		ctx.JSON(200, map[string]interface{}{
			"documents": docs,
			"count":     result.Count,
		})
	}
}

// ---- Batch Write Handler ----

// handleBatchWrite handles POST /db/_batch
func handleBatchWrite(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		var body struct {
			Writes []struct {
				Operation  string                 `json:"operation"` // "set", "update", "delete"
				Collection string                 `json:"collection"`
				DocumentID string                 `json:"document_id"`
				Data       map[string]interface{} `json:"data,omitempty"`
			} `json:"writes"`
		}

		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		if len(body.Writes) == 0 {
			ctx.Error(400, "No writes specified")
			return
		}

		if len(body.Writes) > 500 {
			ctx.Error(400, "Maximum 500 writes per batch")
			return
		}

		batch := engine.NewBatch()
		for _, w := range body.Writes {
			switch w.Operation {
			case "set":
				batch.Set(w.Collection, w.DocumentID, w.Data)
			case "update":
				batch.Update(w.Collection, w.DocumentID, w.Data)
			case "delete":
				batch.Delete(w.Collection, w.DocumentID)
			default:
				ctx.Error(400, fmt.Sprintf("Unknown operation: %s", w.Operation))
				return
			}
		}

		if err := batch.Commit(); err != nil {
			ctx.Error(500, "Batch write failed: "+err.Error())
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"message":    "Batch write successful",
			"operations": len(body.Writes),
		})
	}
}

// ---- Transaction Handler ----

// handleTransaction handles POST /db/_transaction
func handleTransaction(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		var body struct {
			Reads []struct {
				Collection string `json:"collection"`
				DocumentID string `json:"document_id"`
			} `json:"reads"`
			Writes []struct {
				Operation  string                 `json:"operation"`
				Collection string                 `json:"collection"`
				DocumentID string                 `json:"document_id"`
				Data       map[string]interface{} `json:"data,omitempty"`
			} `json:"writes"`
		}

		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		tx := engine.BeginTransaction()

		// Perform reads
		readDocs := make([]map[string]interface{}, 0, len(body.Reads))
		for _, r := range body.Reads {
			doc, err := tx.Get(r.Collection, r.DocumentID)
			if err != nil {
				ctx.Error(500, "Transaction read failed: "+err.Error())
				return
			}
			if doc != nil {
				readDocs = append(readDocs, doc.ToJSON())
			} else {
				readDocs = append(readDocs, map[string]interface{}{
					"_id":        r.DocumentID,
					"_not_found": true,
				})
			}
		}

		// Schedule writes
		for _, w := range body.Writes {
			var err error
			switch w.Operation {
			case "set":
				err = tx.Set(w.Collection, w.DocumentID, w.Data)
			case "update":
				err = tx.Update(w.Collection, w.DocumentID, w.Data)
			case "delete":
				err = tx.Delete(w.Collection, w.DocumentID)
			default:
				tx.Abort()
				ctx.Error(400, fmt.Sprintf("Unknown operation: %s", w.Operation))
				return
			}
			if err != nil {
				tx.Abort()
				ctx.Error(500, "Transaction write scheduling failed: "+err.Error())
				return
			}
		}

		// Commit
		if err := tx.Commit(); err != nil {
			ctx.Error(409, "Transaction conflict: "+err.Error())
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"message":    "Transaction committed",
			"reads":      readDocs,
			"operations": len(body.Writes),
		})
	}
}

// ---- Index Management ----

// handleCreateIndex handles POST /db/_indexes
func handleCreateIndex(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		var body struct {
			Collection string `json:"collection"`
			Field      string `json:"field"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		if body.Collection == "" || body.Field == "" {
			ctx.Error(400, "Collection and field are required")
			return
		}

		engine.CreateIndex(body.Collection, body.Field)

		ctx.JSON(201, map[string]interface{}{
			"message":    "Index created",
			"collection": body.Collection,
			"field":      body.Field,
		})
	}
}

// handleListIndexes handles GET /db/_indexes/:collection
func handleListIndexes(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		collection := ctx.Param("collection")
		fields := engine.ListIndexes(collection)

		ctx.JSON(200, map[string]interface{}{
			"collection": collection,
			"indexes":    fields,
		})
	}
}

// ---- Snapshot & Stats ----

// handleCreateSnapshot handles POST /db/_snapshot
func handleCreateSnapshot(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		if err := engine.CreateSnapshot(); err != nil {
			ctx.Error(500, "Failed to create snapshot: "+err.Error())
			return
		}
		ctx.JSON(200, map[string]interface{}{
			"message": "Snapshot created",
		})
	}
}

// handleStats handles GET /db/_stats
func handleStats(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		ctx.JSON(200, engine.Stats())
	}
}
