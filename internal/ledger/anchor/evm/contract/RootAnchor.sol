// SPDX-License-Identifier: Apache-2.0
pragma solidity 0.8.30;

/// @title RootAnchor
/// @notice An append-only register of daily Merkle roots.
///
/// One publisher — the account that deployed the contract — records, per
/// calendar day, the Merkle root of that day's immunization-event leaves.
/// A day may receive several roots over time as late events are committed;
/// every version is kept and none can be altered or removed. What is stored
/// is a 32-byte commitment over a whole day and nothing else: no leaf, no
/// leaf hash, no count, nothing that describes a person.
///
/// Days are passed as the ASCII date "YYYY-MM-DD" right-padded with zero
/// bytes to 32 bytes, so a day can be checked from any EVM tool without a
/// custom encoder:
///
///   cast call <address> "rootOf(bytes32)(bytes32)" \
///       $(cast format-bytes32-string 2026-08-07)
///
/// The contract deliberately has no owner transfer, no pause, no upgrade
/// path and no way to delete: the only state transition is "append a root
/// for a day", and only the publisher may perform it. Storage is written
/// with plain variables (no immutables), so the deployed runtime bytecode is
/// byte-for-byte the compiler's output and can be verified with eth_getCode.
contract RootAnchor {
    /// @notice The only account allowed to anchor. Fixed at deployment.
    address public publisher;

    /// @dev day => every root anchored for that day, oldest first.
    mapping(bytes32 => bytes32[]) private history;

    /// @notice Emitted once per new root. `version` is 1 for the first root
    /// recorded for a day, 2 for the second, and so on.
    event Anchored(bytes32 indexed day, bytes32 root, uint256 version);

    error NotPublisher();
    error EmptyRoot();

    constructor() {
        publisher = msg.sender;
    }

    /// @notice Append `root` as the newest root for `day`.
    /// @dev Idempotent: anchoring the root that is already newest for the
    /// day changes nothing and emits nothing, so a retried transaction after
    /// a lost receipt cannot inflate the version count.
    function anchor(bytes32 day, bytes32 root) external {
        if (msg.sender != publisher) revert NotPublisher();
        if (root == bytes32(0)) revert EmptyRoot();
        bytes32[] storage h = history[day];
        if (h.length != 0 && h[h.length - 1] == root) return;
        h.push(root);
        emit Anchored(day, root, h.length);
    }

    /// @notice The newest root anchored for `day`, or zero if none.
    function rootOf(bytes32 day) external view returns (bytes32) {
        bytes32[] storage h = history[day];
        if (h.length == 0) return bytes32(0);
        return h[h.length - 1];
    }

    /// @notice How many roots have been anchored for `day`.
    function versions(bytes32 day) external view returns (uint256) {
        return history[day].length;
    }

    /// @notice The root anchored for `day` as version `version` (1-based).
    /// Reverts when no such version exists.
    function rootAt(bytes32 day, uint256 version) external view returns (bytes32) {
        return history[day][version - 1];
    }
}
