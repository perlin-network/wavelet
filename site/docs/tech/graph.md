---
id: graph
title: Graph
sidebar_label: Graph
---

#### General information
Graph ([graph.go](https://github.com/perlin-network/wavelet/blob/master/graph.go)) is the module which holds [DAG](https://en.wikipedia.org/wiki/Directed_acyclic_graph) implementation used in the Wavelet.

[Graph](https://github.com/perlin-network/wavelet/blob/master/graph.go#L74) is main structure which holds all DAG related information and implements all the functionality.
All public/exported API is concurrently safe.

`Graph` stores transactions in couple of different indices - in maps for quick access and some in balanced trees for quick search (e.g. parents and critical transactions).

#### Adding transactions
There are 3 reasons why transaction won't be added and action will result with error:
- transaction already exists in the graph
- depth of the transaction is below threshold (smaller than graph's root depth - [MaxDepthDiff](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L62)) - which basically means that transaction is too old for current graph's state
- transaction is not valid (see [Transaction validation](#transactionvalidation))

If none of the above is true, then transaction will be added to one of the indices, which won't affect graph structure, call it temporary transactions.
In order for transaction to be part of the graph it should be "complete" - graph should have all of the parents of the transaction and they should be "complete" as well. This validation is performed only for transaction with depth below root depth by exactly [MaxParentsPerTransaction](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L65) rounds.
Doing so we allow for some "old" transactions to be added without requirements for parent to be present and complete.
All of the missing or incomplete parents will be stored as missing transactions (see downloading transactions). And for those which present in the graph, they will be stored in `children` index.
If at least one of the ancestors is missing, transaction will be stored in `incomplete` index and error will be returned.
And otherwise transaction will be added to the graph.

##### Updating graph with transaction
Firstly all of the transaction parents are validated (see [Transaction parents validation](#transactionparentsvalidation))
If any is not valid - transaction and its children deleted from all indices of the graph.
Graph height may be updated if new transaction's depth is bigger.
After that transaction will be added to search indexes (b-trees for parent and critical transactions). And for all existing and incomplete children of the transaction we need to check other parents for presence and completion. This is needed because new transaction may be the only one which is preventing child transaction from being complete. 
In such case child transaction will be marked as complete and added to the graph as well.

##### Algorithm
1. Check if transaction already exists - return error if so
2. Check if transaction depth is below root depth by `MaxDepthDiff` round - return error if so
3. Validate transaction, return error if not valid
4. Store transaction in one of the indices, which just holds information about transaction without affecting actual graph
5. If transaction depth plus `MaxParentsPerTransaction` equals to root depth then check for all parents to be present and complete
    * if parent is missing - save it to "missing" index
    * if parent is present and complete - save it and transaction to "children" index
    * if at least one of the parents is missing or incomplete - save transaction to incomplete index and return respective error 
6. Validate transaction parents
    * if they are not valid - delete transaction and its parents from all indices and return error
7. Update graph height if its lower than transaction depth + 1
8. Save transaction to other indices (parents, critical etc)
9. Check if there are incomplete children and check their parents
    * delete them from incomplete index and add to the graph in case new transaction was last missing parent

#### Transaction validation
Here are conditions which needs to be met in order for transaction to be valid (for each case respective error will be returned if its not satisfied):
- it should not have zero/empty id (see transaction id)
- it should not have zero/empty sender and creator
- it should have at least one and at most `MaxParentsPerTransaction` parents
- it should not include itself in the parents, parents should be lexicographically sorted and there should be no duplicated parents
- it should have known tag - if its nop transaction it should have no payload, otherwise there should be non empty payload
- sender and creator (if they aren't the same) should have correct signatures

##### Transaction parents validation
First of all if transaction's depth is below root depth by `MaxDepthDiff` rounds we do not check its parents at all.
Then if there is a parent with depth lower than transaction one by `MaxDepthDiff` round, such parent is considered invalid and error is returned.
If transaction's depth is not bigger exactly by one than max depth of parents - then such parents considered invalid. 
In the opposite case - transaction considered as the one which has valid parents.

#### Parents selection
As was mentioned in the begining, potential parents transaction are stored in separate index - balanced tree in particular. Transaction are sorted by depth and in case of equal depth - by seed length ([seed](#transactionseed)).
Tree is traversed in descending order, i.e. higher depth goes first. First `MaxParentsPerTransaction` number of eligible parents will be selected as parents for new transaction. If parent has depth lower than graph by `MaxDepthDiff` rounds or has children which present in the graph and "complete" such parent won't be taken and instead will be removed from parents index in the graph.
In other words we prioritise "leaf" transactions to be parents.  

##### Algorithm
1. Descend eligible parents balanced tree
    * if there is transaction found which has lower depth than graph height by `MaxDepthDiff` round it should not be selected as parent and deleted from parents index
    * if there is transaction found which has children and at least one of them is neither missing nor incomplete, such transaction should not be selected as parent and should be deleted from parents index
2. Add transaction to selected parents
3. If there are exactly `MaxParentsPerTransaction` parents found then return those, otherwise continue to #1 

#### Critical transaction selection
Potential [critical transactions](#transactioncriticality) stored in separate index in form of balanced tree, same way as eligible parent transactions - sorted by depth first and seed length second.
Tree is traversed in ascending order, i.e. lower depth and higher seed len first. First critical transaction with depth higher than root's will be taken. Meanwhile if there will be found transactions during traversal which have depth lower or equal than root's or which aren't critical for given difficulty, they will be removed from this index.

##### Algorithm
1. Ascend eligible critical balanced tree
    * if transaction depth is lower or equal than root depth - such transaction cannot be selected as critical and should be deleted from critical index
    * if transaction is not critical for given difficulty - such transaction cannot be selected as critical and should be deleted from critical index
2. Return found critical transaction or nil if its not found

#### Transaction Seed
Transaction [seed](https://github.com/perlin-network/wavelet/blob/master/tx.go#L55) its a value which is computed as [blake2b](https://en.wikipedia.org/wiki/BLAKE_(hash_function)#BLAKE2) 256 hash checksum from transaction sender id and all parent transaction ids.

Transaction [seed length](https://github.com/perlin-network/wavelet/blob/master/tx.go#L56) is computed as a number of leading zero bits in transaction seed. 

#### Transaction Criticality
Transaction is considered [critical](https://github.com/perlin-network/wavelet/blob/master/tx.go#L279) for given difficulty if its seed length is greater or equal to given difficulty.