import QtQuick
import QtTest
import "../components/format.js" as Format

TestCase {
    name: "Format"

    function test_format_bytes_zero() {
        compare(Format.formatBytes(0), "0 B")
    }

    function test_format_bytes_bytes() {
        compare(Format.formatBytes(500), "500.0 B")
        compare(Format.formatBytes(1023), "1023.0 B")
    }

    function test_format_bytes_kilobytes() {
        compare(Format.formatBytes(1024), "1.0 KB")
        compare(Format.formatBytes(1536), "1.5 KB")
    }

    function test_format_bytes_megabytes() {
        compare(Format.formatBytes(1048576), "1.0 MB")
        compare(Format.formatBytes(1572864), "1.5 MB")
    }

    function test_format_bytes_gigabytes() {
        compare(Format.formatBytes(1073741824), "1.0 GB")
    }

    function test_format_bytes_terabytes() {
        compare(Format.formatBytes(1099511627776), "1.0 TB")
    }

    function test_format_bytes_clambs_to_max_unit() {
        // 1024 TB stays at TB (the largest unit).
        compare(Format.formatBytes(1024 * 1024 * 1024 * 1024 * 1024), "1024.0 TB")
    }

    function test_format_bytes_invalid() {
        compare(Format.formatBytes(-1), "0 B")
        compare(Format.formatBytes(NaN), "0 B")
        compare(Format.formatBytes(undefined), "0 B")
    }

    function test_format_percent_normal() {
        compare(Format.formatPercent(0), "0.0%")
        compare(Format.formatPercent(4.2), "4.2%")
        compare(Format.formatPercent(100), "100.0%")
    }

    function test_format_percent_clamps() {
        compare(Format.formatPercent(-5), "0.0%")
        compare(Format.formatPercent(150), "100.0%")
    }

    function test_format_percent_invalid() {
        compare(Format.formatPercent(NaN), "\u2014")
        compare(Format.formatPercent(undefined), "\u2014")
        compare(Format.formatPercent(null), "\u2014")
    }

    function test_format_sampled_at() {
        compare(Format.formatSampledAt(""), "\u2014")
        compare(Format.formatSampledAt("not-a-date"), "\u2014")
        compare(Format.formatSampledAt(undefined), "\u2014")
    }

    function test_format_sampled_at_just_now() {
        var now = new Date();
        var iso = now.toISOString();
        // A timestamp from within the last minute is "just now".
        compare(Format.formatSampledAt(iso), "just now");
    }

    function test_format_sampled_at_minutes_ago() {
        // Two minutes ago.
        var past = new Date(Date.now() - 150 * 1000);
        compare(Format.formatSampledAt(past.toISOString()), "2 m ago");
    }
}
