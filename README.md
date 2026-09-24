# 🍃 Go RAG

Proyek iseng-serius buat belajar **RAG (Retrieval-Augmented Generation)** pakai Go murni — nggak pake LangChain, nggak pake Python. Ceritanya: ada katalog produk di database, terus kita bikin bot yang bisa dichat buat nanya-nanya soal produk itu ("Berapa harga Es Teh?", "Ada rekomendasi menu pedas?") — jawabannya diambil dari data asli, bukan ngarang sendiri.

> Kenapa Go doang, nggak pake LangGraph dkk? Karena emang keputusannya gitu dari awal. `StateGraph`-nya LangGraph cuma ada di Python/JS, jadi di sini workflow-nya ditulis manual pakai Go biasa (lihat [`internal/core/rag/graph.go`](internal/core/rag/graph.go)) — logikanya setara, cuma nggak manggil library-nya. Detail lengkapnya ada di [`docs/rag.md`](docs/rag.md).

---

## 🧠 Alur singkatnya

```
Katalog produk (Postgres)
        │
        ▼
  ingest / embed  ──►  pgvector (tabel rag_chunks)
        │
        ▼
  user chat "berapa harga X?"
        │
        ▼
  klasifikasi intent → ambil fakta / cari chunk mirip → susun jawaban
        │
        ▼
  balik ke user + simpan riwayat percakapan (rag_turns)
```

Dua proses utama di proyek ini:

1. **Ingestion** — baca katalog produk, bersihin HTML-nya, potong jadi chunk kecil, terus di-embed (diubah jadi vektor) dan disimpan di Postgres pakai `pgvector`.
2. **Percakapan** — user tanya sesuatu, sistem klasifikasi dulu itu pertanyaan jenis apa (cari fakta harga? cari info naratif? minta rekomendasi? bandingin dua produk?), baru cari data yang relevan dan susun jawaban.

---

## 🗂️ Struktur folder (yang penting-penting aja)

```
cmd/rag/                     → entrypoint aplikasi (main.go) + perintah embed-catalogs
internal/
  ├─ configs/                → baca .env, validasi config
  ├─ core/
  │   ├─ embedding/          → client embedding, splitter teks, engine sync ke DB
  │   └─ rag/                → "otak" chat: graph/state machine, store pgvector, client LLM
  ├─ domain/
  │   ├─ models/             → struct tabel (catalog, rag_chunk, rag_turn, dll)
  │   ├─ repositories/       → akses data (belum semua kepake, lihat catatan di bawah)
  │   └─ services/           → container service
  ├─ database/
  │   ├─ migrations/         → migration manual (bukan pakai lib migrate)
  │   └─ seeders/            → seed data awal
  └─ infrastructure/         → nyalain semuanya: server gin, db, logger, session
docs/rag.md                  → penjelasan detail teknis ingestion & chat workflow (Bahasa Indonesia)
```

> ⚠️ **Catatan jujur:** beberapa file di `internal/handler/api/controllers`, `internal/handler/api/routes`, dan sebagian `internal/domain/repositories` itu sisa eksperimen yang belum dicolok ke `app.go` (pemanggilannya masih di-comment). Rute yang beneran jalan sekarang cuma didefinisikan langsung inline di [`internal/infrastructure/app.go`](internal/infrastructure/app.go). Jangan bingung kalau nemu kode yang "nganggur".

---

## 🚀 Cara jalanin

### 1. Siapin dulu

- Go 1.26+
- PostgreSQL yang sudah aktif ekstensi **pgvector**
- API key buat provider embedding (`nvidia` atau `openai`) dan LLM chat

### 2. Bikin file `.env`

Isi minimal yang dibutuhin (lihat [`internal/configs/base.config.go`](internal/configs/base.config.go) buat daftar lengkap):

```env
GIN_URL=127.0.0.1
GIN_PORT=8080
GIN_MODE=debug

DB_CONNECTION=postgres
DB_HOST=127.0.0.1
DB_PORT=5432
DB_USERNAME=postgres
DB_PASSWORD=secret
DB_DATABASE=go_rag

EMBEDDING_PROVIDER=nvidia
EMBEDDING_BASE_URL=https://integrate.api.nvidia.com/v1
EMBEDDING_API_KEY=xxxxx
EMBEDDING_MODEL=nvidia/nemotron-3-embed-1b

LLM_BASE_URL=https://integrate.api.nvidia.com/v1
LLM_API_KEY=xxxxx
LLM_MODEL=nama-model-chat-kamu
```

### 3. Jalanin (ada `Makefile`, tinggal pilih)

```bash
make run          # jalanin langsung
make air          # jalanin pakai hot-reload (air)
make build        # build binary production
make test         # jalanin test
```

### 4. Sinkronisasi katalog jadi vektor

Wajib dijalanin dulu sebelum nge-chat, biar ada data buat dicari:

```bash
go run ./cmd/rag embed-catalogs
```

Perintah ini pinter — kalau cuma harga/timestamp yang berubah, dia nggak akan panggil ulang API embedding (`embedded=0`), cuma update metadata. Baru re-embed beneran kalau `intro`/`description`-nya berubah. Mau lihat log detailnya? Set `EMBEDDING_SYNC_DEBUG=true` di `.env`.

### 5. Coba chat-nya

```bash
curl -X POST http://127.0.0.1:8080/api/rag/chat \
  -H "Content-Type: application/json" \
  -d '{"conversation_id":"", "message":"Berapa harga Es Teh?"}'
```

Responsnya bakal ada `conversation_id` baru (simpen ini kalau mau lanjut ngobrol), `answer`, `facts`, `retrieved_chunks`, `citations`, sampai `trace` biar kelihatan node mana aja yang dilewatin.

Endpoint lain buat cek-cek doang:

```bash
GET /healthz       # cek app hidup atau nggak
GET /api/version   # cek versi app
```

---

## 🧩 Tech stack

| Bagian | Pakai |
|---|---|
| Web framework | [Gin](https://github.com/gin-gonic/gin) |
| ORM | [GORM](https://gorm.io/) + `pgvector` |
| Tokenizer (buat chunking) | `tiktoken-go` (BPE `cl100k_base`) |
| Logger | `zerolog` + `lumberjack` (rotasi file log) |
| Session | `gin-contrib/sessions` |
| Provider embedding & LLM | NVIDIA NIM (default) atau OpenAI-compatible |

---

## 📖 Baca lebih dalam

Semua detail teknis — cara chunking, format `chunk_id`, alur klasifikasi intent, sampai kenapa milih pgvector dibanding FAISS — ada di [`docs/rag.md`](docs/rag.md). Tulisannya lumayan panjang tapi worth it kalau mau ngerti "kenapa"-nya, bukan cuma "caranya".
