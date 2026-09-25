# RAG katalog produk

## Ingestion

Untuk mengindeks dokumen pengetahuan, simpan berkas `.md`, `.txt`, PDF berbasis teks, `.docx`, `.xlsx`, atau `.csv` di `docs/knowledge/`, lalu jalankan `go run ./cmd/rag embed-docs` dari root proyek. Untuk folder lain gunakan `go run ./cmd/rag embed-docs --dir <path>`. Folder dipindai rekursif. PDF dipecah per halaman, DOCX dibaca per paragraf, dan CSV/XLSX dibaca per baris dengan nama kolom sebagai konteks. Teks panjang dipecah menjadi chunk maksimal `EMBEDDING_CHUNK_SIZE` token (default 500). Perintah berikutnya memakai ulang embedding yang tidak berubah dan menghapus chunk untuk berkas yang sudah dihapus pada folder yang sama. PDF memerlukan `pdftotext` (Poppler) di `PATH`; PDF hasil scan memerlukan OCR lebih dulu. Rumus XLSX memakai nilai tersimpan di berkas, bukan dihitung ulang. Dokumen faktual dapat dipakai untuk pertanyaan knowledge, sedangkan rekomendasi menu mencari produk katalog.

Aturan chatbot disimpan sebagai Markdown di `docs/guardrails/`. File `.md` di folder itu dibaca menurut urutan nama saat CLI dimulai atau setiap request HTTP. Teks Markdown menjadi instruksi tambahan untuk model. Blok kode `guardrail` berisi JSON yang dijalankan aplikasi: `input_rules` menolak pertanyaan sebelum model dipanggil, `max_distance` menolak hasil pencarian knowledge terbuka yang terlalu jauh, dan `output_rules` memeriksa jawaban sebelum ditampilkan. Ambang jarak tidak diterapkan untuk rekomendasi atau produk yang disebutkan secara spesifik agar menu yang relevan tidak tertolak hanya karena skor embedding. Setiap aturan memakai regex Go dan dapat memberi `response` tetap. Nilai `off_topic_answer`, `not_found_answer`, `max_query_chars`, dan `max_answer_chars` juga dapat diatur. Contoh ada di `docs/guardrails/waiter.md`; JSON yang rusak menyebabkan chat gagal dengan error, sehingga aturan tidak diam-diam diabaikan. `docs/knowledge/menyapa.md` lama tetap dilewati saat embedding karena berisi instruksi; isinya sekarang diwakili oleh `docs/guardrails/waiter.md`.

Jalankan `go run ./cmd/rag embed-catalogs` setelah PostgreSQL dan ekstensi pgvector tersedia. Perintah ini membaca produk aktif, termasuk kategori utama, kategori tambahan, dan induk kategori. Jika `intro` dan `description` berbeda, keduanya digabung menjadi satu dokumen naratif dengan penanda `Ringkasan` dan `Deskripsi`. Jika keduanya sama setelah pembersihan HTML, hanya isi `description` yang diindeks seperti sebelumnya. Metadata `covered_fields` mencatat bahwa dokumen tersebut mewakili keduanya. Jika hanya salah satu field yang berisi, dokumen memakai field itu saja. Judul, kategori, dan nama bagian diulang sebagai header setiap chunk; harga, SKU, ID, dan tag disimpan sebagai metadata. Belum ada sumber `labels` tersendiri dalam tabel katalog, sehingga tidak ada label baru yang ditambahkan ke embedding. HTML dan paragraf `<p>-</p>` dibersihkan sebelum embedding.

Untuk menguji dampak perubahan harga, masukkan `EMBEDDING_SYNC_DEBUG=true` pada `.env.local`, ubah `price` dan `updated_at` satu produk, lalu jalankan `go run ./cmd/rag embed-catalogs`. Contoh hasilnya:

```text
[embedding-sync] catalog_id=42 updated_at=2026-09-24T10:30:00Z price=25000 previous_price=20000 price_changed=true chunks=1 embedded=0 reused=1 metadata_updated=1 deleted=0 action=metadata_only
```

`embedded=0` membuktikan harga dan timestamp hanya memperbarui metadata. Jika `intro` atau `description` berubah, `embedded` akan lebih dari nol dan `action=reembedded_narrative`.

Splitter memakai tokenizer BPE `cl100k_base` untuk menghitung token dan separator bertingkat `\n\n`, `\n`, `. `, spasi, lalu token. Nilai awal `EMBEDDING_CHUNK_SIZE=500` dan `EMBEDDING_CHUNK_OVERLAP=75`. Header ikut dalam batas 500 token dan diulang pada setiap chunk; narasi yang lebih panjang dipecah otomatis. Jika header sendiri menyisakan terlalu sedikit ruang untuk isi dan overlap, sinkronisasi mengembalikan error. Tokenizer ini dipakai untuk perkiraan ukuran, bukan tokenizer asli NVIDIA; 500 token berada jauh di bawah batas input model 4096 token.

Setiap chunk mempunyai `chunk_id` deterministik dari sumber, field, dan urutan chunk, serta `content_hash` SHA-256. Sinkronisasi ulang hanya memanggil provider untuk konten atau model yang berubah. Chunk lama untuk produk/field yang hilang dihapus; perubahan harga atau SKU hanya mengubah metadata. Produk nonaktif dan dihapus dibersihkan dari indeks.

Provider embedding diatur melalui `EMBEDDING_PROVIDER`, `EMBEDDING_BASE_URL`, `EMBEDDING_API_KEY`, dan `EMBEDDING_MODEL`. `nvidia` adalah default dan mengirim `input_type=passage` untuk ingestion serta `input_type=query` untuk pencarian; `openai` mengirim format embeddings kompatibel OpenAI tanpa `input_type`. Model NVIDIA yang dipilih saat implementasi ialah `nvidia/nemotron-3-embed-1b`, dengan output **2048 dimensi**. Lihat [referensi model NVIDIA](https://docs.api.nvidia.com/nim/reference/nvidia-nemotron-3-embed-1b) dan [API embedding](https://docs.api.nvidia.com/nim/reference/nvidia-nemotron-3-embed-1b-infer). Jika model diganti, jalankan sinkronisasi ulang agar vektor lama diganti; pencarian hanya memakai baris dengan `embedding_model` yang sesuai.

## Workflow percakapan

Untuk chat langsung dari terminal, jalankan:

```bash
go run ./cmd/rag chat
```

CLI menampilkan `session_id` UUIDv7 saat dimulai. Ketik pertanyaan per baris dan `/exit` untuk keluar. Setiap jawaban dan pertanyaan disimpan di `rag_turns` memakai session ID tersebut. Sebelum setiap pertanyaan diproses, CLI memuat seluruh riwayat session itu dalam urutan percakapan dan memasukkannya ke panggilan LLM untuk klasifikasi serta jawaban. Untuk melanjutkan session yang sudah memiliki giliran tersimpan pada proses berikutnya, jalankan `go run ./cmd/rag chat --session-id <UUIDv7>`. Tanpa opsi itu, setiap proses CLI membuat session baru. CLI menggunakan provider embedding dan LLM yang sama dengan endpoint HTTP. Riwayat yang terus bertambah akhirnya dapat melewati batas konteks provider LLM; saat ini belum ada peringkasan otomatis.

`POST /api/rag/chat` menerima JSON:

```json
{"conversation_id":"", "message":"Berapa harga Es Teh?"}
```

Respons berisi `conversation_id` baru bila kosong, `answer`, `facts`, `retrieved_chunks`, `citations`, `model_used`, `fallback_reason`, dan `trace` node. Kirim ulang `conversation_id` untuk memakai lima giliran terakhir. Model chat dikonfigurasi melalui `LLM_BASE_URL`, `LLM_API_KEY`, dan `LLM_MODEL`; endpoint memakai API chat completions yang kompatibel dengan NVIDIA.

Workflow Go bertipe `State` dengan node `load_conversation`, `classify_intent`, `resolve_entities`, `load_facts`, `retrieve_knowledge`, `score_candidates`, `compose_answer`, `clarify`, dan `persist_turn`. Klasifikasi meminta JSON terstruktur dan dapat merujuk judul produk yang tidak ambigu dari riwayat saat pengguna bertanya lanjutan. `FACT_LOOKUP` mengambil fakta database; `KNOWLEDGE_QA` mengambil chunk naratif; `RECOMMENDATION` mengurutkan kandidat berdasarkan jarak vektor lalu mengambil fakta terbaru; `COMPARE` dan `CROSS_CATEGORY` menggabungkan fakta dan chunk. Judul ambigu atau informasi kurang diarahkan ke klarifikasi. Percakapan disimpan di `rag_turns` tanpa vector memory atau graph persistence.

Permintaan rekomendasi eksplisit tetap diproses sebagai `RECOMMENDATION` walau klasifikasi model meminta klarifikasi tanpa alasan yang jelas. Kata `minuman` menjadi kelompok pencarian yang mencakup kategori katalog aktif seperti `MOCKTAILS`, teh, kopi, jus, dan kategori minuman lain yang dikenal; nama kategori aslinya tetap dipakai sebagai filter metadata. Jika tidak ada kategori minuman yang cocok, hasil pencarian kosong sehingga menu makanan tidak direkomendasikan. Kata rasa seperti `asam` dan `segar` ditambah padanan Inggris hanya pada query embedding. Saat menjawab, chatbot memilih menu yang cocok berdasarkan fakta dan deskripsi, boleh menyebut perkiraan rasa dari bahan yang tercatat dengan penanda yang jelas, serta tidak menampilkan stok atau harga yang tidak ditanya.

Proyek tetap **Go saja** sesuai keputusan implementasi. Karena `StateGraph` LangGraph tersedia untuk Python/JavaScript, routing bertipe di sini adalah implementasi Go dengan fungsi setara, bukan pemanggilan library LangGraph. Lihat [dokumentasi LangGraph](https://docs.langchain.com/oss/javascript/langgraph/thinking-in-langgraph).

`Document` di paket Go menyimpan teks dan metadata yang setara dengan `langchain_core.documents.Document`. Chunking tidak memakai `CharacterTextSplitter` karena batasnya dihitung dalam karakter, sedangkan kebutuhan di sini adalah token. Penyimpanan memakai pgvector pada PostgreSQL yang sudah dipakai aplikasi; FAISS akan membuat indeks terpisah yang perlu disinkronkan lagi dengan data katalog.
