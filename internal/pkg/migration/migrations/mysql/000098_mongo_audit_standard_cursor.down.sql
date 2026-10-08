-- Never run old application audit readers after retiring their source stores.
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'retain standard audit checkpoint; use compatible application rollback';
