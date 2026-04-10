// Native toString masking — we do NOT proxy Function.prototype.toString
// via its utils.patchToString mechanism. We do NOT add another proxy layer on top
// because double-wrapping breaks CreepJS's chain-cycle TypeError check, causing
// lieProps['Function.toString'] to be set → hasToStringProxy = true (detected).
//
