package ftp

const (
	replyGroupPreliminaryReply             = 1
	replyGroupPositiveCompletion           = 2
	replyRestartMarker                     = 110
	replyReadyInNMinutes                   = 120
	replyDataConnectionAlreadyOpen         = 125
	replyFileStatusOkay                    = 150
	replyCommandOkay                       = 200
	replyCommandOkayNotImplemented         = 202
	replySystemStatus                      = 211
	replyDirectoryStatus                   = 212
	replyFileStatus                        = 213
	replyHelpMessage                       = 214
	replySystemType                        = 215
	replyServiceReady                      = 220
	replyClosingControlConnection          = 221
	replyDataConnectionOpen                = 225
	replyClosingDataConnection             = 226
	replyEnteringPassiveMode               = 227
	replyEnteringExtendedPassiveMode       = 229
	replyUserLoggedIn                      = 230
	replyAuthOkayNoDataNeeded              = 234
	replyFileActionOkay                    = 250
	replyDirCreated                        = 257
	replyNeedPassword                      = 331
	replyNeedAccount                       = 332
	replyFileActionPending                 = 350
	replyServiceNotAvailable               = 421
	replyCantOpenDataConnection            = 425
	replyConnectionClosed                  = 426
	replyTransientFileError                = 450
	replyLocalError                        = 451
	replyOutOfSpace                        = 452
	replyCommandSyntaxError                = 500
	replyParameterSyntaxError              = 501
	replyCommandNotImplemented             = 502
	replyBadCommandSequence                = 503
	replyCommandNotImplementedForParameter = 504
	replyNotLoggedIn                       = 530
	replyNeedAccountToStore                = 532
	replyFileError                         = 550
	replyPageTypeUnknown                   = 551
	replyExceededStorageAllocation         = 552
	replyBadFileName                       = 553
)

func positiveCompletionReply(code int) bool {
	return code/100 == 2
}

func positivePreliminaryReply(code int) bool {
	return code/100 == 1
}

func transientNegativeCompletionReply(code int) bool {
	return code/100 == 4
}
