import QtQuick
import QtTest
import "../components/backoff.js" as Backoff

TestCase {
    name: "Backoff"

    function test_first_failure_waits_one_second() {
        compare(Backoff.delayForAttempt(0), 1000)
    }

    function test_delay_doubles() {
        compare(Backoff.delayForAttempt(1), 2000)
        compare(Backoff.delayForAttempt(2), 4000)
        compare(Backoff.delayForAttempt(3), 8000)
        compare(Backoff.delayForAttempt(4), 16000)
    }

    function test_delay_is_capped_at_thirty_seconds() {
        compare(Backoff.delayForAttempt(5), 30000)
        compare(Backoff.delayForAttempt(6), 30000)
        compare(Backoff.delayForAttempt(20), 30000)
    }

    function test_invalid_attempt_uses_minimum() {
        compare(Backoff.delayForAttempt(-1), 1000)
        compare(Backoff.delayForAttempt(Number.NaN), 1000)
    }
}
