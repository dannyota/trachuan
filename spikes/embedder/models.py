"""Throwaway spike: candidate embedding models, pinned, with their documented prompts.

Prompts come from each model card or its config_sentence_transformers.json, read
2026-11-09. Qwen3 uses banhmi's legal instruction; base Harrier gets the same
instruction in Harrier's "Instruct: ...\\nQuery: " form.
"""

from dataclasses import dataclass

LEGAL_TASK = "Given a legal or regulatory query, retrieve relevant provisions and regulatory text that address the query"


@dataclass(frozen=True)
class Spec:
    key: str
    model_id: str
    revision: str
    query_prompt: str
    doc_prompt: str = ""
    max_len: int = 1024
    remote_code: bool = False
    note: str = ""


SPECS = [
    Spec("qwen3", "Qwen/Qwen3-Embedding-0.6B", "97b0c614be4d",
         "Instruct: " + LEGAL_TASK + "\nQuery:", note="baseline, Apache-2.0, 2025-06"),
    Spec("vi_v2", "AITeamVN/Vietnamese_Embedding_v2", "18b44161e041", "", max_len=2048,
         note="BGE-M3 fine-tune, Apache-2.0"),
    Spec("harrier", "microsoft/harrier-oss-v1-0.6b", "f9b9dc8d367d",
         "Instruct: " + LEGAL_TASK + "\nQuery: ", note="MIT, 2026-03"),
    Spec("vietlegal_harrier", "mainguyen9/vietlegal-harrier-0.6b", "91a0e1ebe4b6",
         "Instruct: Given a Vietnamese legal question, retrieve relevant legal passages that answer the question\nQuery: ",
         max_len=512, note="Apache-2.0, Harrier fine-tune on Vietnamese law, 2026-04"),
]
BY_KEY = {s.key: s for s in SPECS}

# Dropped by the owner on 2026-11-09 to shorten the screening round; not measured.
NOT_RUN = ["ibm-granite/granite-embedding-311m-multilingual-r2", "MongoDB/voyage-4-nano", "google/embeddinggemma-2", "perplexity-ai/pplx-embed-v1-0.6b", "nvidia/Nemotron-3-Embed-1B-BF16"]
