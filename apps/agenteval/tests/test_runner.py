import asyncio
import json
from decimal import Decimal
from types import SimpleNamespace

from trachuan_agenteval.agent import Outcome
from trachuan_agenteval.runner import execute
from trachuan_agenteval.usage import Budget, Price, Usage


def test_scheduler_charges_each_request_finishes_task_and_marks_budget_incomplete(tmp_path):
    calls = []

    async def task(provider, mcp, tools, question, mode, max_turns, max_tool_calls, on_usage):
        calls.append(mode)
        on_usage(Usage(600_000))
        on_usage(Usage(10_000))
        return Outcome(usage=Usage(610_000), stop_reason="final")

    matcher = SimpleNamespace(score=lambda *args: {"matched": 0, "expected": 1})
    budget = Budget(Decimal("1"))
    summary = asyncio.run(
        execute(
            [{"id": "fixture", "question": "never write this question"}],
            ["fake"],
            ["full", "compact"],
            2,
            {"fake": object()},
            {"full": (object(), []), "compact": (object(), [])},
            {"fake": Price(2, 10, 0.1, 2.5)},
            budget,
            matcher,
            tmp_path,
            4,
            10,
            task=task,
        )
    )
    assert calls == ["full"]
    assert summary["status"] == "incomplete"
    assert summary["cost_usd"] == "1.22"
    assert summary["models"]["fake"]["gates"]["compact_vs_full"] == "unavailable"
    records = [json.loads(line) for line in (tmp_path / "tasks.jsonl").read_text().splitlines()]
    assert len(records) == 1
    assert records[0]["summed_input_tokens"] == 610_000
    assert "never write" not in (tmp_path / "tasks.jsonl").read_text()


def test_provider_error_stops_scheduling_because_spend_may_be_unknown(tmp_path):
    async def task(*args, **kwargs):
        return Outcome(stop_reason="provider_error")

    matcher = SimpleNamespace(score=lambda *args: {"matched": 0, "expected": 1})
    result = asyncio.run(
        execute(
            [{"id": "fixture", "question": "fixture"}],
            ["fake"],
            ["full", "compact"],
            1,
            {"fake": object()},
            {"full": (object(), []), "compact": (object(), [])},
            {"fake": Price(2, 10, 0.1, 2.5)},
            Budget(Decimal(60)),
            matcher,
            tmp_path,
            4,
            10,
            task=task,
        )
    )
    assert result["scheduled_tasks"] == 1
    assert result["status"] == "incomplete"
