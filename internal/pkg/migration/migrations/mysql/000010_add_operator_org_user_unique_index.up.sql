ALTER TABLE `staff`
    ADD CONSTRAINT `uk_staff_org_user` UNIQUE (`org_id`, `user_id`);
