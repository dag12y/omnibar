import QtQuick
import QtTest
import "../components/snapshots.js" as Snapshots

TestCase {
    name: "Snapshots"
    function test_live_reading_replaces_snapshot_without_mutating_cache() {
        const cached = {cpu: {percent: 1}, memory: {used: 50, total: 100}}
        const next = Snapshots.applySystemEvent(cached, "cpu", {percent: 75})
        verify(next !== cached)
        compare(cached.cpu.percent, 1)
        compare(next.cpu.percent, 75)
        compare(next.memory.used, 50)
        verify(next.sampled_at.length > 0)
    }
    function test_memory_event_updates_swap() {
        const next = Snapshots.applySystemEvent({}, "memory", {used: 20, total: 100, swap_total: 10, swap_used: 2})
        compare(next.memory.used, 20)
        compare(next.swap.total, 10)
        compare(next.swap.used, 2)
    }
}
