---
id: graph
title: Graph
sidebar_label: Graph
---

## General information
**[graph.go](https://github.com/perlin-network/wavelet/blob/master/graph.go)** is the module which holds the [DAG](https://en.wikipedia.org/wiki/Directed_acyclic_graph) implementation used in **Wavelet**.

`Graph` is the main structure which holds all DAG related information.
All public/exported API is concurrently safe.

`Graph` stores transactions in a couple of different indices. Maps are utilised for quick access and balanced trees are used for quick searching (e.g. parents and critical transactions).

## Adding transactions
There are 3 reasons why **AddTransaction** might return an error:
- the transaction already exists in the graph.
- the depth of the transaction is below a threshold (smaller than graph's root depth, by [`maxDepthDiff`](https://github.com/perlin-network/wavelet/blob/master/conf/conf.go)) meaning that the transaction is too old for current graph's state.
- the transaction is [not valid](#transaction-validation).

If none of the above are true, then the transaction is added to one of the indices which will not affect graph structure. This is referred to as a temporary transaction. And deleted from `missing` transactions index. Transaction also added to CLI auto-completion indexer if such preset.

In order for a transaction to be part of the graph it must be **complete**. This means that the graph should have all of the parents of the transaction and that they should be **complete** as well. This validation is performed for transactions which are at least one level above `maxDepthDiff` threshold. Which means that if transaction is exactly at this level it can be safely added to the graph without parents. 

Those with missing or incomplete parents will be stored as missing transactions. Those which are present in the graph, are  stored in the `children` index.

If at least one of the ancestors is missing, the transaction will be stored in the `incomplete` index and an error will be returned.
Otherwise, the transaction will be added to the graph.
All missing parents will be stored in `missing` transactions index, assuming that parent transaction's depth is one below of it's child.

### Updating graph with transaction
Firstly, all of the transaction parents are [validated](#transaction-parents-validation).

If any are found to be invalid, the transaction and its children are deleted from all indices of the graph.
The graph height may be updated if the new transaction's depth is bigger.

After that, the transaction will be added to search indexes (b-trees for parent and critical transactions) and its children will be [resolved](#resolving-children).

### Resolving Children
`Resolving` transaction's children means that for all of it's existing and incomplete children, other parents will be checked for presence and completion. This is required because new transaction may be the only one thing which is preventing a child transaction from being completed. 
In such a case, a child transaction will be marked as completed and added to the graph as well.

### Algorithm
1. Check if transaction already exists and return an error if this is the case
2. Check if transaction depth is below the root depth by `maxDepthDiff` levels and return an error if this is the case
3. Validate the transaction and return an error if invalid
4. Store the transaction in one of the indices which holds information about the transaction without altering the actual graph
5. If the transaction depth plus `maxDepthDiff` does not equal to root depth then check for all parents to be present and complete
    - if the parent is missing, save the it to the "missing" index assuming its depth one level less than it's child
    - if the parent is present and complete - save both it and the transaction to the "children" index
    - if at least one of the parents is missing or incomplete, save the transaction to the incomplete index and return respective error 
6. Validate transaction parents
    - if invalid, delete both the transaction and its parents from all indices and return an error
7. Update the graph height if it is lower than the transaction depth + 1
8. Save the transaction to other indices (parents, critical etc)
9. Check if there are incomplete children and check their parents
    * delete them from the incomplete index and add them to the graph in case a new transaction was the last missing parent

## Transaction validation
In **validateTransaction**, the following conditions need to be met. For each case, a respective error will be returned in the event the condition is not satisfied:
- it should not have zero/empty id (see transaction id)
- it should not have zero/empty sender and creator
- it should have at least one and at most `maxParentsPerTransaction` parents
- it should not include itself in the parents, parents should be lexicographically sorted and there should be no duplicated parents
- it should have a known tag - if it is a nop transaction it should have no payload, otherwise there should be a non-empty payload
- sender and creator, if they are not the same, should have correct signatures

### Transaction parents validation
The following describes the behavior of **validateTransactionParents**.

First of all, if a transaction's depth is below the root depth by `maxDepthDiff` levels we do not check its parents at all.

If there is non existing parent, error returned.

If there is a parent with a depth lower than the transaction's depth by `maxDepthDiff` levels, such parent is considered invalid and an error is returned.

If transaction's depth is not larger than max depth of parents by exactly one then such parents are considered invalid.

In the opposite case, the transaction is considered as the one which has valid parents.

## Parents selection
As was mentioned earlier, potential parents for transactions are stored in balanced trees located at a separate index. Transactions are sorted by depth and, in the case of equal depth, by [seed length](#transaction-seed).

The tree is traversed in descending order, i.e. higher depths are traversed first. 

First `maxParentsPerTransaction` number of eligible parents will be selected as parents for a new transaction. If a parent has depth lower than the graph by `maxDepthDiff + 1` levels or has children which are present in the graph and are "complete", such parent will not be selected and instead will be removed from the parents index in the graph.

In other words, "leaf" transactions are prioritised when selecting parents.  

### Algorithm
1. Descend eligible parents balanced tree
    - if there is a transaction found which has lower depth than the graph height by `maxDepthDiff` levels it should not be selected as a parent and is instead deleted from parents index
    - if there is transaction found which has children and at least one of them is neither missing nor incomplete, such a transaction should not be selected as the parent and should be deleted from the parents index
2. Add the transaction to selected parents
3. If there are exactly `maxParentsPerTransaction` parents found then return those, otherwise continue to 1.

## Critical transaction selection
Potential [critical transactions](#transaction-criticality) are stored in a separate index in the form of a balanced tree. These are stored in the same way as eligible parent transactions, that is, sorted by depth first and seed length second.

The tree is traversed in ascending order, i.e. lower depth and higher seed length are traversed first. 

First, critical transactions with a depth higher than the root's will be selected. If there are transactions found during this traversal which have a depth lower or equal to the root's or which aren't critical for given difficulty, they will be removed from this index.

### Algorithm
1. Ascend eligible critical balanced tree
    - if transaction depth is lower or equal to the root depth, such a transaction cannot be selected as critical and should be deleted from the critical index
    - if transaction is not critical for given difficulty, such a transaction cannot be selected as critical and should be deleted from the critical index
2. Return found critical transaction or nil if it is not found

## Transaction Seed
Transaction [seed](https://github.com/perlin-network/wavelet/blob/master/tx.go#L57) is a value which is computed as a [blake2b](https://en.wikipedia.org/wiki/BLAKE_(hash_function)#BLAKE2) 256 hash checksum using the transaction sender id and all parent transaction ids.

Transaction [seed length](https://github.com/perlin-network/wavelet/blob/master/tx.go#L58) is computed as the number of leading zero bits in a transaction seed. 

## Transaction Criticality
A transaction is considered [critical](https://github.com/perlin-network/wavelet/blob/master/tx.go#L290) for a given difficulty if its seed length is greater or equal to the given difficulty.

## Pruning Transactions
[Pruning transactions](https://github.com/perlin-network/wavelet/blob/master/graph.go) means to delete from all indices the transactions below the given depth. 
This is required in the case of a new state of the ledger (new round), either after syncing or after consensus has been achieved. 

For ease of deleting transactions from the graph by given depth, transaction ids are stored in a separate index by depth.
Now, when needed, transaction ids are selected according to the given depth and deleted from all other indexes and depth index.
Both missing and incomplete transactions are checked separately (since they aren't present in depth index) afterwards and removed as well.

The Number of pruned transactions is then returned.

## Updating Graph's Root
Updating graph's root happens after syncing and building new state of the ledger.
New root will be added to all: depth, eligible parents' and main transactions indexes.
Graph's height will be updated as height of new root + 1.
After that root's depth will be updated.

### Updating Graph Root's Depth
This operation happens either within updating root operation (after syncing) or by itself after finalization.

Updating root's depth includes traversing missing and incomplete transactions indexes and deleting all transactions with depth lower than root's by `maxDepthDiff` levels. For each deleted missing or incomplete transaction it's children will be [resolved](#resolving-children). This makes sense because if parent, previously missing or incomplete, is evicted from the graph because of it's depth and any of it's children is below new root's depth by exactly `maxDepthDiff` levels it now can be added to the graph, because it may have no parents (see adding transaction to the graph).  

Same traversal and deletion will be applied for eligible parents and critical transactions indexes. 