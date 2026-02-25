DROP INDEX IF EXISTS vectors_term_chunk_idx;
DROP INDEX IF EXISTS vectors_tfidf_idx;
DROP INDEX IF EXISTS vectors_term_idx;
DROP INDEX IF EXISTS vectors_chunk_id_idx;
DROP TABLE IF EXISTS document_vectors;

DROP INDEX IF EXISTS chunks_type_idx;
DROP INDEX IF EXISTS chunks_file_id_idx;
DROP TABLE IF EXISTS chunks;

DROP INDEX IF EXISTS files_hash_idx;
DROP INDEX IF EXISTS files_path_idx;
DROP TABLE IF EXISTS files;
